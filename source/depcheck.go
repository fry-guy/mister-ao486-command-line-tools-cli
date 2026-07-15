package main

import (
	"os/exec"
)

// depCheck describes one external tool or data file aotools shells
// out to / reads, but does not and cannot bundle itself (see the
// long comment on checkDependencies for why). fetchURL, when
// non-empty, points at a known community-hosted copy `aotools
// install` can offer to download on the user's explicit request --
// see fetch.go.
type depCheck struct {
	label      string
	path       string
	hint       string
	fetchURL   string
	executable bool
}

// qemuBiosSentinel marks the one depCheck entry (the qemu-bios
// directory) that isn't a single-file fetch -- it's fetched as a
// whole directory of files, handled specially in fetch.go.
const qemuBiosSentinel = "QEMU_BIOS_DIR"

// fixedPathDeps lists every fixed-path dependency from paths.go that
// aotools reads or execs but never creates itself. Where a
// known-good community-hosted copy exists, fetchURL lets `aotools
// install` offer to fetch it (gated behind an explicit copyright
// acknowledgment -- these are never aotools's own files to give
// away).
var fixedPathDeps = []depCheck{
	{"mtools", mtoolsBin, "needed by every VHD/floppy command (vhd create, vhd resize, mgl create, ima create)",
		"https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/mtools", true},
	{"chdman", chdmanBin, "needed by `chd create` and `mount chd`",
		"https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/chdman", true},
	{"qemu-system-i386", qemuBin, "needed by `vhd create -dos` and `vhd create -win31`",
		"https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/qemu-system-i386", true},
	{"qemu BIOS directory", qemuBiosDir, "QEMU's own standard bundled data dir -- comes with qemu-system-i386 automatically, not a separate download",
		qemuBiosSentinel, false},
	{"DOS boot floppy (disk1.img)", floppySrc, "needed by `vhd create -dos` to drive the real FORMAT+XCOPY install",
		"https://archive.org/download/ms-dos-6.22-with-enchanced-tools-floppy-disks_20231027/MS-DOS%206.22%20with%20Enchanced%20Tools%20Floppy%20Disks.7z/MS-DOS%206.22%20with%20Enchanced%20Tools%20Floppy%20Disks%2FMS-DOS%206.22%20with%20Enchanced%20Tools%20Floppy%20Disks%2FDisk1.img", false},
	{"DOS VHD template", dosTemplate, "needed by `vhd create -dos` for its DOS/MISTER/DRIVERS/UTIL overhead",
		"https://archive.org/download/dos-6.22_202607/dos_template.vhd", false},
	{"Windows 3.1 VHD template", win31Template, "needed by `vhd create -win31`",
		"https://archive.org/download/win31_template/win31_template.vhd", false},
}

// pathDeps are external binaries expected on $PATH rather than at a
// fixed aotools-specific location. There's no sensible single-file
// download for any of these (they're base OS / busybox-level
// tools), so they're never offered as auto-fetchable.
var pathDeps = []string{"mkfs.vfat", "mount", "losetup"}

// checkDependencies reports on every external tool/data file aotools
// depends on but doesn't (and, for licensing and size reasons,
// shouldn't) bundle itself:
//
//   - qemu-system-i386, chdman, and mtools are third-party
//     pre-compiled binaries that any working install of the
//     *original* ao486 DOS Toolkit already required, before aotools
//     existed. aotools didn't add this dependency; it just automated
//     driving them. Embedding a redistributed copy of any of these in
//     this binary would both bloat it hugely and raise licensing
//     questions that are someone else's to answer, not this
//     project's.
//   - dos_template.vhd/win31_template.vhd/disk1.img are DOS/Windows
//     3.1 system disk images. The Windows 3.1 template alone is
//     realistically tens to hundreds of MB, and more importantly
//     contains actual copyrighted Microsoft system files -- not
//     something any tool has the right to bundle and redistribute,
//     aotools included.
//
// So `aotools install` can make itself fully self-contained (it's
// one binary, one command), but it can never make the surrounding
// ao486 DOS Toolkit environment self-contained -- that was never
// true even before this port existed. What it CAN do, and does here,
// is check for all of it up front and report exactly what's missing
// and why, instead of letting a user hit a confusing failure deep
// inside `vhd create -dos` five minutes later. Where a known-good
// community-hosted copy exists, `aotools install` separately offers
// to fetch it on request -- see fetch.go.
func checkDependencies() bool {
	missing, _, allOK := checkDependenciesDetailed()
	_ = missing
	return allOK
}

// checkDependenciesDetailed does the same reporting as
// checkDependencies but also hands back which items are missing (and
// whether qemu-bios specifically is one of them), so cmdInstall can
// go on to offer downloading them.
func checkDependenciesDetailed() (missing []depCheck, qemuBiosMissing bool, allOK bool) {
	eprintln("Checking for required external tools and data files...")
	eprintln()
	allOK = true
	for _, c := range fixedPathDeps {
		if fileExists(c.path) {
			eprintf("  [ok]      %-28s %s\n", c.label, c.path)
		} else {
			allOK = false
			eprintf("  [MISSING] %-28s %s\n", c.label, c.path)
			eprintf("            %s\n", c.hint)
			if c.fetchURL == qemuBiosSentinel {
				qemuBiosMissing = true
			} else {
				missing = append(missing, c)
			}
		}
	}
	for _, bin := range pathDeps {
		if _, err := exec.LookPath(bin); err == nil {
			eprintf("  [ok]      %-28s (found on PATH)\n", bin)
		} else {
			allOK = false
			eprintf("  [MISSING] %-28s not found on PATH\n", bin)
		}
	}
	eprintln()
	if allOK {
		eprintln("Everything aotools depends on is present.")
	} else {
		eprintln("aotools itself is a single file and is fully installed. The items")
		eprintln("marked MISSING above are separate from aotools -- they're the same")
		eprintln("external MiSTer tools and DOS/Windows 3.1 media the original ao486")
		eprintln("DOS Toolkit always required. aotools can't bundle or provide them")
		eprintln("(qemu/chdman/mtools are external packages with their own licensing;")
		eprintln("the VHD templates and boot floppy contain real, copyrighted")
		eprintln("DOS/Windows system files aotools has no right to redistribute).")
		eprintln("Commands that don't need a missing item above will still work fine")
		eprintln("-- e.g. `mgl create` doesn't need qemu or chdman at all.")
	}
	return missing, qemuBiosMissing, allOK
}

// cmdDoctor implements `aotools doctor`: a standalone, read-only way
// to re-run the same dependency check `install` does, any time,
// without touching user-startup.sh and without offering to download
// anything (that offer is deliberately only part of `install` --
// see fetch.go).
func cmdDoctor() {
	checkDependencies()
}
