#!/usr/bin/env python3
"""
Write a proper DOS-compatible MBR and partition table to a raw disk image.
This replaces sfdisk for creating VHDs that DOS/SeaBIOS can recognize.
"""
import struct, sys, os

def make_mbr(vhd_path, size_mb):
    """Write a proper MBR with FAT16 partition to a VHD file"""
    
    total_bytes = size_mb * 1024 * 1024
    total_sectors = total_bytes // 512
    
    # Partition starts at sector 128 (legacy CHS alignment)
    part_start = 128
    part_size = total_sectors - part_start
    
    # Calculate CHS for partition start and end
    heads = 16
    spt = 63
    
    def lba_to_chs(lba):
        c = lba // (heads * spt)
        h = (lba // spt) % heads
        s = (lba % spt) + 1
        if c > 1023:
            c, h, s = 1023, 254, 63  # cap at max CHS
        return c, h, s
    
    sc, sh, ss = lba_to_chs(part_start)
    ec, eh, es = lba_to_chs(part_start + part_size - 1)
    
    # Build partition entry (16 bytes)
    # Byte 0: boot indicator (0x80 = bootable)
    # Bytes 1-3: CHS of first sector
    # Byte 4: partition type (0x06 = FAT16 LBA)
    # Bytes 5-7: CHS of last sector
    # Bytes 8-11: LBA start (little-endian)
    # Bytes 12-15: LBA size (little-endian)
    
    chs_start = bytes([
        sh,
        (ss & 0x3f) | ((sc >> 2) & 0xc0),
        sc & 0xff
    ])
    chs_end = bytes([
        eh,
        (es & 0x3f) | ((ec >> 2) & 0xc0),
        ec & 0xff
    ])
    
    partition_entry = bytes([0x80]) + chs_start + bytes([0x06]) + chs_end + \
                     struct.pack('<II', part_start, part_size)
    
    # MBR: 446 bytes boot code + 4x16 byte partition entries + 2 byte signature
    # Real bootstrap: relocates itself to 0000:0600, scans the partition
    # table for the active (0x80) entry, loads that partition's boot
    # sector via INT13h extended LBA read (falling back to CHS read if
    # the BIOS lacks extensions), verifies its 0x55AA signature, and
    # chainloads it with DL=drive, DS:SI=partition entry (standard
    # legacy DOS MBR convention). Source: mbr2.asm (NASM).
    BOOT_CODE_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mbr_bootstrap.bin")
    with open(BOOT_CODE_PATH, "rb") as bf:
        boot_code = bf.read()
    assert len(boot_code) == 446, f"boot code must be exactly 446 bytes, got {len(boot_code)}"
    
    partition_table = partition_entry + bytes(16 * 3)  # 3 empty partitions
    signature = bytes([0x55, 0xAA])
    
    mbr = boot_code + partition_table + signature
    assert len(mbr) == 512
    
    # Write MBR to the VHD
    with open(vhd_path, 'r+b') as f:
        f.seek(0)
        f.write(mbr)
    
    print(f"MBR written: partition at sector {part_start}, size {part_size} sectors")
    print(f"CHS start: {sc}/{sh}/{ss}")
    print(f"CHS end: {ec}/{eh}/{es}")
    print(f"MBR signature: 0x55 0xAA ✓")

if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <vhd_path> <size_mb>")
        sys.exit(1)
    make_mbr(sys.argv[1], int(sys.argv[2]))
