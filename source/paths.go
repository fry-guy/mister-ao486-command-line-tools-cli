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
	imaMountPoint    = "/tmp/ima_mount"

	qmpSock = "/tmp/ao486-qmp.sock"

	// userStartupPath is the one persistent hook MiSTer runs at every
	// boot (via /etc/init.d/S99user), on /media/fat so it survives
	// reboots. IMPORTANT: it runs once, as its own standalone child
	// process of the init system -- any `export`/`eval` done inside
	// it dies with that process and is NOT inherited by SSH sessions
	// opened later. So `aotools install` doesn't try to export/eval
	// anything there directly; instead it has user-startup.sh
	// (re)write profileDPath (below) on every boot, which IS sourced
	// by every login shell via /etc/profile's own profile.d loop.
	userStartupPath = "/media/fat/linux/user-startup.sh"

	// profileDPath is what actually reaches every new interactive
	// shell: /etc/profile sources every *.sh file in /etc/profile.d/
	// for every login shell. This directory lives on the root
	// filesystem, NOT on /media/fat, so anything written here does
	// NOT survive a reboot on its own -- that's exactly why
	// user-startup.sh (persistent) recreates this file fresh on
	// every single boot rather than relying on it staying put.
	profileDPath = "/etc/profile.d/aotools.sh"

	// Partition geometry used throughout: every VHD this toolkit builds
	// starts its single FAT12/16 partition at LBA 128 (legacy CHS
	// alignment), with a fixed 16 heads / 63 sectors-per-track geometry.
	partStartSector = 128
	geomHeads       = 16
	geomSPT         = 63
)
