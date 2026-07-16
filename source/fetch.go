package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// qemuBiosFiles is the exact file list QEMU's own standard pc-bios
// share directory ships with (SeaBIOS, VGA BIOS variants, network
// boot ROMs, etc.), mirrored here so `aotools install` can offer to
// fetch the whole directory one file at a time without needing to
// call the GitHub API (and hit its rate limits) just to list a
// directory that never changes. Verified against both a real
// deployed qemu-bios/ folder and the upstream repo listing.
var qemuBiosFiles = []string{
	"QEMU,cgthree.bin",
	"QEMU,tcx.bin",
	"bios-256k.bin",
	"bios-microvm.bin",
	"bios.bin",
	"efi-e1000.rom",
	"efi-e1000e.rom",
	"efi-eepro100.rom",
	"efi-ne2k_pci.rom",
	"efi-pcnet.rom",
	"efi-rtl8139.rom",
	"efi-virtio.rom",
	"efi-vmxnet3.rom",
	"kvmvapic.bin",
	"linuxboot.bin",
	"linuxboot_dma.bin",
	"multiboot.bin",
	"multiboot_dma.bin",
	"npcm7xx_bootrom.bin",
	"opensbi-riscv32-generic-fw_dynamic.bin",
	"opensbi-riscv64-generic-fw_dynamic.bin",
	"pvh.bin",
	"pxe-e1000.rom",
	"pxe-eepro100.rom",
	"pxe-ne2k_pci.rom",
	"pxe-pcnet.rom",
	"pxe-rtl8139.rom",
	"pxe-virtio.rom",
	"qboot.rom",
	"slof.bin",
	"u-boot-sam460-20100605.bin",
	"vgabios-ati.bin",
	"vgabios-bochs-display.bin",
	"vgabios-cirrus.bin",
	"vgabios-qxl.bin",
	"vgabios-ramfb.bin",
	"vgabios-stdvga.bin",
	"vgabios-virtio.bin",
	"vgabios-vmware.bin",
	"vgabios.bin",
	"vof-nvram.bin",
	"vof.bin",
}

const qemuBiosBaseURL = "https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/linux/qemu-bios/"

// downloadFile fetches url to dest via curl (not Go's net/http --
// this MiSTer's buildroot has no CA certificate bundle installed, so
// TLS verification needs the same -k/--insecure treatment the
// original Go-toolchain fetch in INSTALL.md already relies on).
// Downloads to a .partial sibling first and renames into place only
// on full success, so a failed/interrupted download never leaves a
// half-written file at the real destination.
func downloadFile(url, dest string, executable bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".partial"
	out, err := runCapture("curl", "-k", "-fsSL", "-o", tmp, url)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("curl failed: %s", strings.TrimSpace(out))
	}
	fi, statErr := os.Stat(tmp)
	if statErr != nil || fi.Size() == 0 {
		os.Remove(tmp)
		return fmt.Errorf("downloaded file is empty or missing")
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	if executable {
		_ = os.Chmod(dest, 0755)
	}
	return nil
}

// sttyRun runs `stty` with the given args against the real controlling
// terminal (cmd.Stdin is explicitly wired to os.Stdin, since os/exec
// otherwise gives the child no stdin at all, which would make `stty`
// fail immediately). Used to save/restore terminal settings around
// waitEnterOrEsc's raw single-keypress read.
func sttyRun(args ...string) (string, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// waitEnterOrEsc waits for a single keypress: Enter proceeds (true),
// Esc cancels (false), anything else is ignored and it keeps waiting.
// Requires putting the terminal into raw mode (via `stty`) so a
// single keypress is delivered immediately instead of waiting for a
// newline -- settings are always restored afterward.
//
// Falls back to a plain line read (empty line proceeds, anything else
// cancels) when stdin isn't a real terminal -- e.g. scripted/piped
// input -- since raw mode has nothing to operate on in that case.
func waitEnterOrEsc() bool {
	saved, err := sttyRun("-g")
	if err != nil || saved == "" {
		line := promptLine("")
		return strings.TrimSpace(line) == ""
	}
	if _, err := sttyRun("raw", "-echo"); err != nil {
		line := promptLine("")
		return strings.TrimSpace(line) == ""
	}
	defer sttyRun(saved)

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return false // EOF/error -- treat as cancel
		}
		switch buf[0] {
		case '\r', '\n':
			return true
		case 27: // Esc
			return false
		}
		// any other key: ignore, keep waiting
	}
}

// offerFetchMissing is the download half of `aotools install`. It
// only ever runs after checkDependenciesDetailed has already shown
// the user exactly what's missing -- this is strictly an opt-in
// convenience on top of that, never a silent background action, and
// never offered from `aotools doctor` (which stays purely read-only).
//
// Community installers for MiSTer (update_all.sh and others) use
// this same pattern for content they don't have the right to bundle:
// ask, then require a clear acknowledgment that it's someone else's
// copyrighted work, then fetch only on confirmation.
func offerFetchMissing(missing []depCheck, qemuBiosMissing bool) {
	if len(missing) == 0 && !qemuBiosMissing {
		return
	}

	eprintln("aotools is currently installed but lacks some key functionalities due to missing assets. The items marked MISSING cannot be packaged in aotools directly because they are separate packages with their own licensing or contain copyrighted material. This installer can instead download these resources from known community-hosted copies.")
	answer := promptLine("Download the missing item(s) now? [y/N]: ")
	if !strings.EqualFold(answer, "y") {
		eprintln("Skipped. Run `aotools install` again any time to retry.")
		return
	}

	eprintln("=======================================================")
	eprintln("IMPORTANT - PLEASE REVIEW BEFORE PROCEEDING")
	eprintln("=======================================================")
	eprintln("By continuing, you acknowledge these are third-party files subject")
	eprintln("to their own licenses/copyright, downloaded at your own request and")
	eprintln("placed on your own device -- aotools is only automating the fetch,")
	eprintln("the same as other community installers such as update_all.sh do for content")
	eprintln("they don't have the right to bundle themselves.")
	eprintln("Please select Enter to continue, or esc to cancel.")

	if !waitEnterOrEsc() {
		eprintln("Cancelled. Nothing was downloaded.")
		return
	}

	for _, m := range missing {
		eprintf("Downloading %s ...\n", m.label)
		if err := downloadFile(m.fetchURL, m.path, m.executable); err != nil {
			eprintf("  FAILED: %v\n", err)
			continue
		}
		eprintf("  OK -> %s\n", m.path)
	}

	if qemuBiosMissing {
		eprintf("Downloading qemu BIOS directory (%d files) ...\n", len(qemuBiosFiles))
		failures := 0
		for _, name := range qemuBiosFiles {
			// GitHub's own raw URLs percent-encode commas in path
			// segments (seen directly in the repo's API response for
			// "QEMU,cgthree.bin" / "QEMU,tcx.bin") -- matched here
			// rather than relying on a generic path escaper, which
			// may leave the comma unescaped since it's technically a
			// legal literal character in a URL path segment.
			encoded := strings.ReplaceAll(name, ",", "%2C")
			url := qemuBiosBaseURL + encoded
			dest := filepath.Join(qemuBiosDir, name)
			if err := downloadFile(url, dest, false); err != nil {
				eprintf("  FAILED: %s: %v\n", name, err)
				failures++
			}
		}
		if failures == 0 {
			eprintf("  OK -> %s (%d files)\n", qemuBiosDir, len(qemuBiosFiles))
		} else {
			eprintf("  %d of %d files failed -- re-run `aotools install` to retry.\n", failures, len(qemuBiosFiles))
		}
	}

	eprintln("Done. Re-checking...")
	checkDependencies()
}
