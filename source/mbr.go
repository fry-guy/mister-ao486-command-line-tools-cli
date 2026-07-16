package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ---- make_mbr.py port ----------------------------------------------

func lbaToChs(lba int64) (c, h, s int) {
	heads := int64(geomHeads)
	spt := int64(geomSPT)
	cc := lba / (heads * spt)
	hh := (lba / spt) % heads
	ss := (lba % spt) + 1
	if cc > 1023 {
		cc, hh, ss = 1023, 254, 63
	}
	return int(cc), int(hh), int(ss)
}

// makeMBR writes a proper DOS-compatible MBR + single FAT16/FAT12
// partition table entry to vhdPath, exactly mirroring make_mbr.py.
func makeMBR(vhdPath string, sizeMB int64) error {
	totalBytes := sizeMB * 1024 * 1024
	totalSectors := totalBytes / 512

	partStart := int64(partStartSector)
	partSize := totalSectors - partStart

	sc, sh, ss := lbaToChs(partStart)
	ec, eh, es := lbaToChs(partStart + partSize - 1)

	chsStart := []byte{
		byte(sh),
		byte((ss & 0x3f) | ((sc >> 2) & 0xc0)),
		byte(sc & 0xff),
	}
	chsEnd := []byte{
		byte(eh),
		byte((es & 0x3f) | ((ec >> 2) & 0xc0)),
		byte(ec & 0xff),
	}

	partEntry := make([]byte, 16)
	partEntry[0] = 0x80
	copy(partEntry[1:4], chsStart)
	partEntry[4] = 0x06 // FAT16 LBA; fix_bpb corrects this afterward
	copy(partEntry[5:8], chsEnd)
	binary.LittleEndian.PutUint32(partEntry[8:12], uint32(partStart))
	binary.LittleEndian.PutUint32(partEntry[12:16], uint32(partSize))

	bootCode := mbrBootstrap()

	mbr := make([]byte, 0, 512)
	mbr = append(mbr, bootCode...)         // 446 bytes
	mbr = append(mbr, partEntry...)        // 16 bytes, partition 1
	mbr = append(mbr, make([]byte, 16*3)...) // partitions 2-4, empty
	mbr = append(mbr, 0x55, 0xAA)

	if len(mbr) != 512 {
		return fmt.Errorf("internal error: MBR is %d bytes, expected 512", len(mbr))
	}

	f, err := os.OpenFile(vhdPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteAt(mbr, 0); err != nil {
		return err
	}

	eprintf("MBR written: partition at sector %d, size %d sectors\n", partStart, partSize)
	eprintf("CHS start: %d/%d/%d\n", sc, sh, ss)
	eprintf("CHS end: %d/%d/%d\n", ec, eh, es)
	eprintf("MBR signature: 0x55 0xAA\n")
	return nil
}

// ---- fix_bpb.py port -------------------------------------------------

// fixBPB rewrites the geometry-dependent BPB fields (and the MBR
// partition-type byte) after mkfs.vfat has formatted the partition,
// exactly mirroring fix_bpb.py. partStartSec is always
// partStartSector (128) for every VHD this toolkit creates.
func fixBPB(vhdPath string, partStartSec int64, sizeMB int64) error {
	offset := partStartSec * 512
	totalSectors := (sizeMB * 1024 * 1024) / 512
	partSectors := totalSectors - partStartSec

	f, err := os.OpenFile(vhdPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	bs := make([]byte, 512)
	if _, err := f.ReadAt(bs, offset); err != nil {
		return err
	}

	var ts16 uint16
	var ts32 uint32
	if partSectors <= 65535 {
		ts16 = uint16(partSectors)
		ts32 = 0
	} else {
		ts16 = 0
		ts32 = uint32(partSectors)
	}
	binary.LittleEndian.PutUint16(bs[19:21], ts16)
	binary.LittleEndian.PutUint32(bs[32:36], ts32)
	binary.LittleEndian.PutUint32(bs[28:32], uint32(partStartSec)) // HiddenSec
	binary.LittleEndian.PutUint16(bs[24:26], geomSPT)              // SecPerTrack
	binary.LittleEndian.PutUint16(bs[26:28], geomHeads)            // Heads

	if _, err := f.WriteAt(bs, offset); err != nil {
		return err
	}

	fatLabel := bs[0x36:0x3E]
	var partType byte
	var fatTypeStr string
	switch {
	case len(fatLabel) >= 5 && string(fatLabel[:5]) == "FAT12":
		partType = 0x01
		fatTypeStr = "FAT12"
	case ts32 == 0:
		partType = 0x04
		fatTypeStr = "FAT16"
	default:
		partType = 0x06
		fatTypeStr = "FAT16"
	}
	if _, err := f.WriteAt([]byte{partType}, 446+4); err != nil {
		return err
	}

	eprintf("BPB fixed:\n")
	eprintf("  TotalSec16: %d\n", ts16)
	eprintf("  TotalSec32: %d\n", ts32)
	eprintf("  HiddenSec: %d\n", partStartSec)
	eprintf("  SecPerTrack: %d\n", geomSPT)
	eprintf("  Heads: %d\n", geomHeads)
	eprintf("  Partition type: 0x%02x (%s)\n", partType, fatTypeStr)
	return nil
}

// ---- shared FAT boot-sector / root-directory helpers -----------------

// readBootSector reads the 512-byte boot sector at partition offset
// partStartSec (in 512-byte sectors) from path.
func readBootSector(path string, partStartSec int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, 512)
	if _, err := f.ReadAt(buf, partStartSec*512); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeBootSectorMerge preserves the real DOS boot CODE from
// sourceBoot (bytes 0-10: jump + OEM name, bytes 62-511: boot code +
// embedded strings + 0x55AA signature) while keeping the freshly
// written BPB fields (bytes 11-61) already present at the
// destination's partition offset.
func writeBootSectorMerge(destPath string, partStartSec int64, sourceBoot []byte) error {
	if len(sourceBoot) != 512 {
		return fmt.Errorf("source boot sector is not 512 bytes")
	}
	f, err := os.OpenFile(destPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	target := make([]byte, 512)
	if _, err := f.ReadAt(target, partStartSec*512); err != nil {
		return err
	}
	merged := make([]byte, 512)
	copy(merged, target)
	copy(merged[0:11], sourceBoot[0:11])
	copy(merged[62:512], sourceBoot[62:512])

	_, err = f.WriteAt(merged, partStartSec*512)
	return err
}

// readFatAttrManifest scans the root directory of the FAT volume at
// partition offset partStartSec (in 512-byte sectors) and returns a
// map of short filename ("NAME.EXT") -> DOS attribute byte, for every
// entry that has at least one of read-only/hidden/system set.
func readFatAttrManifest(path string, partStartSec int64) (map[string]byte, error) {
	boot, err := readBootSector(path, partStartSec)
	if err != nil {
		return nil, err
	}
	reserved := binary.LittleEndian.Uint16(boot[14:16])
	numFats := boot[16]
	fatSz16 := binary.LittleEndian.Uint16(boot[22:24])
	rootEntries := binary.LittleEndian.Uint16(boot[17:19])
	rootOffsetSectors := int64(reserved) + int64(numFats)*int64(fatSz16)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := make([]byte, int64(rootEntries)*32)
	if _, err := f.ReadAt(data, partStartSec*512+rootOffsetSectors*512); err != nil {
		return nil, err
	}

	manifest := map[string]byte{}
	for i := 0; i < int(rootEntries); i++ {
		e := data[i*32 : (i+1)*32]
		if e[0] == 0x00 {
			break
		}
		if e[0] == 0xE5 {
			continue
		}
		attr := e[11]
		if attr == 0x0F || (attr&0x10) != 0 {
			continue
		}
		if attr&0x07 != 0 {
			name := trimSpaceASCII(string(e[0:8]))
			ext := trimSpaceASCII(string(e[8:11]))
			fname := name
			if ext != "" {
				fname = name + "." + ext
			}
			manifest[fname] = attr
		}
	}
	return manifest, nil
}

func trimSpaceASCII(s string) string {
	i := len(s)
	for i > 0 && (s[i-1] == ' ' || s[i-1] == 0) {
		i--
	}
	j := 0
	for j < i && (s[j] == ' ' || s[j] == 0) {
		j++
	}
	return s[j:i]
}

// mattribFlagsFor returns the mtools mattrib flag set (+r/+h/+s) for
// a captured DOS attribute byte.
func mattribFlagsFor(attr byte) []string {
	var flags []string
	if attr&0x01 != 0 {
		flags = append(flags, "+r")
	}
	if attr&0x02 != 0 {
		flags = append(flags, "+h")
	}
	if attr&0x04 != 0 {
		flags = append(flags, "+s")
	}
	return flags
}

// partitionStartSector reads the LBA start of the first partition
// table entry directly from the MBR (bytes 454-457), used by `mount
// vhd` in place of shelling out to fdisk.
func partitionStartSector(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 16)
	if _, err := f.ReadAt(buf, 446); err != nil {
		return 0, err
	}
	lba := binary.LittleEndian.Uint32(buf[8:12])
	if lba == 0 {
		return 0, fmt.Errorf("no partition found in MBR (or it's not a partitioned image)")
	}
	return int64(lba), nil
}
