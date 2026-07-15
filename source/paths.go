package main

// Fixed install locations, mirroring the README's "File Placement"
// table exactly. This tool is designed to live at
// /media/fat/linux/aotools on a real MiSTer, alongside the existing
// binaries/system files it orchestrates (qemu-system-i386, chdman,
// mtools, the DOS/Win31 templates, etc). Those paths are intentionally
// hardcoded, not configurable -- that's the same assumption every
// original script made.
const (
	linuxDir  = "/media/fat/linux"
	gamesRoot = "/media/fat/games/AO486"

	floppySrc     = "/media/fat/games/ao486/floppy/DOS622/disk1.img"
	dosTemplate   = "/media/fat/games/ao486/dos_template.vhd"
	win31Template = "/media/fat/games/ao486/win31_template.vhd"
	helperImg     = "/media/fat/games/ao486/floppy/format_helper.img"

	qemuBin    = linuxDir + "/qemu-system-i386"
	qemuBiosDir = linuxDir + "/qemu-bios"
	chdmanBin  = linuxDir + "/chdman"
	mtoolsBin  = linuxDir + "/mtools"
	mcopyBin   = linuxDir + "/mcopy"
	mattribBin = linuxDir + "/mattrib"

	bigScratchBase = linuxDir + "/.mkvhd_scratch"

	dosGamesDir   = "/media/fat/_DOS Games"
	win31GamesDir = "/media/fat/_Win 3.1 Games"

	vhdMountPoint    = "/tmp/vhd_mount"
	chdExtractDir    = linuxDir + "/.mountchd_extract"
	chdMountPoint    = linuxDir + "/.mountchd_mount"

	qmpSock = "/tmp/ao486-qmp.sock"

	// userStartupPath is the script MiSTer's own boot process sources
	// on every startup. `aotools install` appends a single `eval`
	// line here (idempotently) so the mountvhd/umountvhd/mountchd/
	// umountchd/mkvhd/... shell functions are available in every new
	// shell without the user having to remember to source anything
	// by hand.
	userStartupPath = "/media/fat/linux/user-startup.sh"

	// Partition geometry used throughout: every VHD this toolkit builds
	// starts its single FAT12/16 partition at LBA 128 (legacy CHS
	// alignment), with a fixed 16 heads / 63 sectors-per-track geometry.
	partStartSector = 128
	geomHeads       = 16
	geomSPT         = 63
)
