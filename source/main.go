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
//	aotools create vhd [-dos|-win31] <name.vhd> [archive]
//	aotools resize vhd <name.vhd>
//	aotools create mgl -dos|-win31 "Display Name" [source_folder]
//	aotools create chd <input.iso|.cue|.bin|.gdi> <output.chd>
//	aotools create diskimage <name.ima> [source] [-s size]
//	aotools mount vhd <name.vhd>
//	aotools umount vhd
//	aotools mount chd <name.chd>
//	aotools umount chd
//	aotools mount diskimage <name.ima>
//	aotools umount diskimage
//	aotools install
//	aotools uninstall
//	aotools shellinit
//
// Every command follows a consistent <verb> <noun> order: create/
// resize/mount/umount always come first, followed by what they act
// on (vhd/mgl/chd/diskimage).
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

// main wraps run() in a recover that catches fatal()'s fatalExit
// panic (see util.go) and only then exits with status 1 -- by that
// point the panic has already unwound the whole call stack, running
// every defer registered along the way (temp/staging directory
// cleanup included). Any panic value that ISN'T a fatalExit is
// re-panicked immediately, so a genuine bug still crashes normally
// with a real stack trace instead of being mistaken for a clean
// "Error: ..." exit.
func main() {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalExit); ok {
				os.Exit(1)
			}
			panic(r)
		}
	}()
	run()
}

func run() {
	if len(os.Args) < 2 {
		printTopUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create":
		if len(os.Args) < 3 {
			printTopUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "vhd":
			cmdVHDCreate(os.Args[3:])
		case "mgl":
			cmdMGLCreate(os.Args[3:])
		case "chd":
			cmdCHDCreate(os.Args[3:])
		case "diskimage":
			cmdIMACreate(os.Args[3:])
		default:
			printTopUsage()
			os.Exit(1)
		}
	case "resize":
		if len(os.Args) < 3 || os.Args[2] != "vhd" {
			printTopUsage()
			os.Exit(1)
		}
		cmdVHDResize(os.Args[3:])
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
		case "diskimage":
			cmdMountIMA(os.Args[3:])
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
		case "diskimage":
			cmdUmountIMA()
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
  aotools create vhd [-dos|-win31] <name.vhd> [archive]
  aotools resize vhd <name.vhd>
  aotools create mgl -dos|-win31 "Display Name" [source_folder]
  aotools create chd <input.iso|.cue|.bin|.gdi> <output.chd>
  aotools create diskimage <name.ima> [source] [-s size]
  aotools mount vhd <name.vhd>
  aotools umount vhd
  aotools mount chd <name.chd>
  aotools umount chd
  aotools mount diskimage <name.ima>
  aotools umount diskimage
  aotools install
  aotools uninstall
  aotools shellinit
  aotools doctor`)
}
