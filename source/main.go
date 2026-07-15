// aotools is a single-binary port of the MiSTer ao486 DOS Toolkit
// (mkvhd, resizevhd, mkmgl, mkchd, mkima, mountvhd/umountvhd,
// mountchd/umountchd, plus the make_mbr.py/fix_bpb.py/
// newgame_controller.py helpers they drove). It's meant to be dropped
// onto a MiSTer at /media/fat/linux/aotools with no installation
// step beyond `chmod +x` -- no compiler, no Python, no extra
// binaries beyond the ones the original toolkit already needed
// (chdman, qemu-system-i386, mtools, mkfs.vfat, losetup/mount, all of
// which are assumed already present at their documented paths).
//
// Usage:
//
//	aotools vhd create [-dos|-win31] <name.vhd> [archive]
//	aotools vhd resize <name.vhd>
//	aotools mgl create -dos|-win31 "Display Name" [source_folder]
//	aotools chd create <input.iso|.cue|.bin|.gdi> <output.chd>
//	aotools ima create <name.ima> [source] [-s size]
//	aotools mount vhd <name.vhd>
//	aotools umount vhd
//	aotools mount chd <name.chd>
//	aotools umount chd
//	aotools install
//	aotools uninstall
//	aotools shellinit
//
// `mount`/`umount` cd you into/out of the mounted volume automatically
// -- but only once `aotools install` (or `eval "$(aotools shellinit)"`)
// has loaded aotools's shell integration into your session. That
// integration makes `aotools` itself a shell function (plus the
// legacy mountvhd/umountvhd/mountchd/umountchd/mkvhd/... shortcuts),
// so `aotools mount vhd x.vhd` and `mountvhd x.vhd` are exactly the
// same thing under the hood. All of this is embedded in this binary
// itself (see shellinit.go) -- there is no separate .sh file to
// install. If you run this binary directly by its full path with no
// shell integration loaded, mount/umount still work but can't cd you
// anywhere (a subprocess can never change its parent shell's own
// working directory -- that's a hard OS limitation).
package main

import (
	"fmt"
	"os"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printTopUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "vhd":
		if len(os.Args) < 3 {
			printTopUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "create":
			cmdVHDCreate(os.Args[3:])
		case "resize":
			cmdVHDResize(os.Args[3:])
		default:
			printTopUsage()
			os.Exit(1)
		}
	case "mgl":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			printTopUsage()
			os.Exit(1)
		}
		cmdMGLCreate(os.Args[3:])
	case "chd":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			printTopUsage()
			os.Exit(1)
		}
		cmdCHDCreate(os.Args[3:])
	case "ima":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			printTopUsage()
			os.Exit(1)
		}
		cmdIMACreate(os.Args[3:])
	case "mount":
		if len(os.Args) < 3 {
			printTopUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "vhd":
			cmdMountVHD(os.Args[3:])
		case "chd":
			cmdMountCHD(os.Args[3:])
		default:
			printTopUsage()
			os.Exit(1)
		}
	case "umount":
		if len(os.Args) < 3 {
			printTopUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "vhd":
			cmdUmountVHD()
		case "chd":
			cmdUmountCHD()
		default:
			printTopUsage()
			os.Exit(1)
		}
	case "shellinit":
		cmdShellInit()
	case "install":
		cmdInstall()
	case "uninstall":
		cmdUninstall()
	case "doctor":
		if !checkDependencies() {
			os.Exit(1)
		}
	case "-v", "--version", "version":
		fmt.Println("aotools " + version)
	case "-h", "--help", "help":
		printTopUsage()
	default:
		printTopUsage()
		os.Exit(1)
	}
}

func printTopUsage() {
	eprintln(`aotools ` + version + ` - MiSTer ao486 DOS Toolkit (single-binary port)

Usage:
  aotools vhd create [-dos|-win31] <name.vhd> [archive]
  aotools vhd resize <name.vhd>
  aotools mgl create -dos|-win31 "Display Name" [source_folder]
  aotools chd create <input.iso|.cue|.bin|.gdi> <output.chd>
  aotools ima create <name.ima> [source] [-s size]
  aotools mount vhd <name.vhd>
  aotools umount vhd
  aotools mount chd <name.chd>
  aotools umount chd
  aotools install
  aotools uninstall
  aotools shellinit
  aotools doctor

Once 'aotools install' has run (or you've loaded shell integration
with 'eval "$(aotools shellinit)"'), 'aotools mount vhd'/'aotools
umount vhd'/'aotools mount chd'/'aotools umount chd' automatically cd
you into and back out of the mounted volume -- 'aotools' itself
becomes a shell function that does this, so there's no separate
syntax to remember. The legacy mountvhd/umountvhd/mountchd/umountchd/
mkvhd/... shortcuts (embedded in this binary, no separate file) do
the exact same thing and share the same state.`)
}
