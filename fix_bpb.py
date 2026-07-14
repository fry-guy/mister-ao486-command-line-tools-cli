#!/usr/bin/env python3
"""
Fix the FAT16 BPB after mkfs.vfat to make it MS-DOS 6.22 compatible.
Sets TotalSec16/TotalSec32 following the standard FAT16 convention (real
MS-DOS FAT16 volumes above 32MB always use TotalSec16=0, TotalSec32=real
count -- this is not DOS-6.22-specific, it's the general FAT16 spec).
Also fixes HiddenSec, SecPerTrack, and Heads to match actual geometry.
"""
import struct, sys

def fix_bpb(vhd_path, part_start_sector, size_mb):
    """Fix BPB fields at partition boot sector"""
    
    offset = part_start_sector * 512
    total_sectors = (size_mb * 1024 * 1024) // 512
    part_sectors = total_sectors - part_start_sector
    
    heads = 16
    spt = 63
    
    with open(vhd_path, 'r+b') as f:
        f.seek(offset)
        bs = bytearray(f.read(512))
        
        # Standard FAT16 BPB convention (used by every real DOS FAT16
        # volume above 32MB since DOS 4.0): if the true sector count
        # fits in the 16-bit TotalSec16 field, use that and zero
        # TotalSec32. Otherwise (partition > 65535 sectors, i.e. >32MB),
        # set TotalSec16=0 and put the real count in TotalSec32 instead.
        #
        # The previous version here unconditionally clamped TotalSec16
        # to min(part_sectors, 65535) and left TotalSec32 untouched
        # (stale/zero) whenever part_sectors exceeded 65535 -- silently
        # capping every VHD above ~32MB to a ~32MB filesystem, since
        # FORMAT's QuickFormat preserves the existing BPB fields rather
        # than recalculating them from the true partition size, so the
        # clamp stuck permanently instead of self-correcting.
        if part_sectors <= 65535:
            ts16 = part_sectors
            ts32 = 0
        else:
            ts16 = 0
            ts32 = part_sectors
        struct.pack_into('<H', bs, 19, ts16)
        struct.pack_into('<I', bs, 32, ts32)
        
        # Fix HiddenSectors - must equal partition start sector
        struct.pack_into('<I', bs, 28, part_start_sector)
        
        # Fix SecPerTrack and Heads to match our geometry
        struct.pack_into('<H', bs, 24, spt)   # 63 sectors/track
        struct.pack_into('<H', bs, 26, heads)  # 16 heads
        
        f.seek(offset)
        f.write(bytes(bs))

        # Fix partition type byte in MBR partition table entry 1.
        # MS-DOS 6.22 doesn't recognize 0x0e (FAT16 LBA) and won't assign
        # a drive letter to it. It needs 0x01 (FAT12), 0x04 (FAT16,
        # volume small enough to use TotalSec16), or 0x06 (FAT16,
        # volume large enough to need TotalSec32) depending on what
        # mkfs.vfat actually wrote. Below ~4085 clusters mkfs.vfat
        # auto-selects FAT12 (this happens on small -g VHDs now that we
        # no longer force -F 16), so the partition type must match or
        # DOS's FAT12/16 handling can mismatch the on-disk structure.
        # Detect FAT12 vs FAT16 from mkfs.vfat's own OEM label at
        # offset 0x36 rather than assuming.
        #
        # The 0x04-vs-0x06 distinction specifically was a latent bug
        # here for a long time: this used to unconditionally emit 0x06
        # for any FAT16 volume, which happens to still work under
        # generic QEMU/SeaBIOS and Linux's own vfat driver (both read
        # the actual BPB rather than strictly trusting the MBR type
        # byte), and on every VHD mkvhd itself builds, DOS's own
        # FORMAT /S silently corrects it to 0x04 for small volumes
        # during install, masking the bug completely. The real ao486
        # core's BIOS is stricter and depends on this byte being
        # correct -- confirmed the hard way via a real hardware boot
        # hang on a VHD built by a tool that (correctly) doesn't run
        # FORMAT /S at all. Match ts16/ts32 (already decided above
        # using the same size threshold) rather than guessing again.
        fat_label = bytes(bs[0x36:0x3E])
        if fat_label.startswith(b'FAT12'):
            part_type = 0x01
            fat_type_str = "FAT12"
        elif ts32 == 0:
            part_type = 0x04
            fat_type_str = "FAT16"
        else:
            part_type = 0x06
            fat_type_str = "FAT16"
        f.seek(446 + 4)
        f.write(bytes([part_type]))

    print(f"BPB fixed:")
    print(f"  TotalSec16: {ts16}")
    print(f"  TotalSec32: {ts32}")
    print(f"  HiddenSec: {part_start_sector}")
    print(f"  SecPerTrack: {spt}")
    print(f"  Heads: {heads}")
    print(f"  Partition type: 0x{part_type:02x} ({fat_type_str})")

if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <vhd_path> <size_mb>")
        sys.exit(1)
    fix_bpb(sys.argv[1], 128, int(sys.argv[2]))
