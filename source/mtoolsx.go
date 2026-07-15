package main

import (
	"fmt"
	"os/exec"
)

// mtoolsImageArg builds the "@@offset"-style image spec mtools uses
// to address a partition inside a raw disk image (e.g. a VHD's FAT
// volume starting at sector 128).
func mtoolsImageArg(path string, offsetBytes int64) string {
	if offsetBytes == 0 {
		return path
	}
	return fmt.Sprintf("%s@@%d", path, offsetBytes)
}

// mcopyPut copies one or more local files/dirs onto an mtools image
// (mcopy -D o -D O -i <image> <src...> ::<dest>), overwriting without
// prompting. dest is appended directly after "::" so callers control
// whether it names an exact destination file (e.g. "AUTOEXEC.BAT"), a
// directory to copy into (e.g. a game folder name), or "" for the
// image root.
func mcopyPut(image string, sources []string, destOnImage string) error {
	args := []string{"-D", "o", "-D", "O", "-i", image}
	args = append(args, sources...)
	args = append(args, "::"+destOnImage)
	cmd := exec.Command(mcopyBin, args...)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy: %v: %s", err, out)
	}
	return nil
}

// mcopyPutRecursive is mcopyPut with -s (recurse into directories),
// used for whole-tree copies directly onto an mtools image (no loop
// mount involved -- used by mkima, which is entirely root-free).
func mcopyPutRecursive(image string, sources []string, destOnImage string) error {
	args := []string{"-s", "-D", "o", "-D", "O", "-i", image}
	args = append(args, sources...)
	args = append(args, "::"+destOnImage)
	cmd := exec.Command(mcopyBin, args...)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy -s: %v: %s", err, out)
	}
	return nil
}

// mcopyOut copies a single file off an mtools image to a local path
// (mcopy -n -i <image> ::<src> <localdest>).
func mcopyOut(image, srcOnImage, localDest string) error {
	cmd := exec.Command(mcopyBin, "-n", "-i", image, "::"+srcOnImage, localDest)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy: %v: %s", err, out)
	}
	return nil
}

// mcopyOutTree recursively copies an entire mtools image's contents
// to a local directory (mcopy -s -i <image> :: <localdir>).
func mcopyOutTree(image, localDest string) error {
	cmd := exec.Command(mcopyBin, "-s", "-i", image, "::", localDest)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy -s: %v: %s", err, out)
	}
	return nil
}

// mattribSet applies hidden/system/read-only flags to a file already
// on an mtools image.
func mattribSet(image, pathOnImage string, flags []string) error {
	if len(flags) == 0 {
		return nil
	}
	args := append(append([]string{}, flags...), "-i", image, "::"+pathOnImage)
	cmd := exec.Command(mattribBin, args...)
	cmd.Stdin = nil
	_, _ = cmd.CombinedOutput() // best-effort, mirrors original's `2>/dev/null` swallow
	return nil
}

// applyAttrManifest restores captured DOS attributes (hidden/system/
// read-only) after a copy that necessarily lost them (a plain Unix
// staging file has no way to represent those bits). prefix, if
// non-empty, is prepended to each manifest key with a "/" separator
// (used when restoring attributes for files injected into a game
// subfolder).
func applyAttrManifest(image string, manifest map[string]byte, prefix string) {
	for fname, attr := range manifest {
		flags := mattribFlagsFor(attr)
		if len(flags) == 0 {
			continue
		}
		p := fname
		if prefix != "" {
			p = prefix + "/" + fname
		}
		_ = mattribSet(image, p, flags)
	}
}
