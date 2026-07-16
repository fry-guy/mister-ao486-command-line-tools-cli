package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// cmdVHDResize implements `aotools resize vhd <name.vhd>`, porting
// resizevhd in full: reads the whole existing VHD into staging,
// builds a fresh correctly-sized container, preserves the source's
// own boot sector code and DOS attribute bytes, copies everything
// back, verifies byte-for-byte via checksum, then replaces the
// original (or keeps it as a timestamped backup).
func cmdVHDResize(args []string) {
	if len(args) == 0 {
		eprintln("Usage: aotools resize vhd <name.vhd>")
		os.Exit(1)
	}
	source := args[0]
	if !fileExists(source) {
		fatal("file not found: %s", source)
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		fatal("%v", err)
	}
	sourceDir := filepath.Dir(abs)
	sourceName := filepath.Base(abs)

	if err := ensureMtoolsSymlinks(); err != nil {
		fatal("%v", err)
	}

	eprintf("Reading %s...\n", sourceName)
	sourceImage := mtoolsImageArg(abs, partStartSector*512)
	stageDir, err := os.MkdirTemp("", "resizevhd_stage_")
	if err != nil {
		fatal("%v", err)
	}
	defer os.RemoveAll(stageDir)
	if err := mcopyOutTree(sourceImage, stageDir); err != nil {
		fatal("failed to read %s: %v", abs, err)
	}
	entries, _ := os.ReadDir(stageDir)
	if len(entries) == 0 {
		fatal("nothing was read from %s -- is it a valid VHD?", abs)
	}

	// Capture the source's own boot sector code and DOS attribute
	// bytes before anything else touches it -- both are lost by a
	// plain Unix staging copy and must be faithfully restored
	// afterward. See writeBootSectorMerge/readFatAttrManifest for why.
	bootOrig, err := readBootSector(abs, partStartSector)
	if err != nil {
		fatal("%v", err)
	}
	attrManifest, err := readFatAttrManifest(abs, partStartSector)
	if err != nil {
		attrManifest = map[string]byte{}
	}

	actualBytes := dirSizeBytes(stageDir)
	fi, _ := os.Stat(abs)
	currentFileMB := mbCeil(fi.Size())

	headroom := actualBytes / 4
	if headroom < 102400 {
		headroom = 102400
	}
	totalBytes := actualBytes + headroom
	suggestedMB := mbCeil(totalBytes)
	if suggestedMB < 2 {
		suggestedMB = 2
	}
	if suggestedMB >= 2048 {
		suggestedMB = 2047
	}

	eprintln()
	eprintf("  Current file size:   %d MB\n", currentFileMB)
	eprintf("  Actual content used: %d KB\n", kbCeil(actualBytes))
	eprintf("  Suggested new size:  %d MB\n", suggestedMB)
	eprintln()

	sizeStr := promptLine(fmt.Sprintf("Enter new size in MB, or 'c' to cancel [%d]: ", suggestedMB))
	if sizeStr == "c" || sizeStr == "C" {
		eprintln("Cancelled; no changes made.")
		return
	}
	sizeMB := suggestedMB
	if sizeStr != "" {
		v, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			fatal("size must be a whole number.")
		}
		sizeMB = v
	}
	if sizeMB < 2 || sizeMB >= 2048 {
		fatal("size must be between 2 and 2047 MB.")
	}
	neededMB := mbCeil(actualBytes)
	if sizeMB < neededMB {
		fatal("%dMB is too small to hold the existing content (~%dMB).", sizeMB, neededMB)
	}

	newVHD := filepath.Join(sourceDir, fmt.Sprintf(".resizevhd_%d.vhd", os.Getpid()))
	eprintln()
	eprintf("Creating %dMB container...\n", sizeMB)
	if err := createSparseFile(newVHD, sizeMB*1024*1024); err != nil {
		fatal("%v", err)
	}
	cleanupNewVHD := true
	defer func() {
		if cleanupNewVHD {
			os.Remove(newVHD)
		}
	}()

	eprintln("Formatting filesystem...")
	if err := formatAndFixBPB(newVHD, sizeMB); err != nil {
		fatal("%v", err)
	}

	eprintln("Writing DOS boot sector (preserving original boot code)...")
	if err := writeBootSectorMerge(newVHD, partStartSector, bootOrig); err != nil {
		fatal("failed to write boot sector: %v", err)
	}

	eprintln("Copying files...")
	// IO.SYS/MSDOS.SYS/COMMAND.COM (if present) get copied FIRST and
	// separately from everything else, directly via mtools (no loop
	// mount) -- mkfs.vfat's own fresh filesystem is otherwise empty,
	// so this claims early, contiguous clusters the way real DOS
	// always does. Doing this after copying everything else pushed
	// IO.SYS to a cluster deep in the disk on real content, which
	// hung on real ao486 hardware despite looking completely valid
	// under every other check.
	sysFiles := []string{"IO.SYS", "MSDOS.SYS", "COMMAND.COM"}
	var sysItems, remainingItems []string
	for _, e := range entries {
		isSys := false
		for _, sf := range sysFiles {
			if strings.EqualFold(e.Name(), sf) {
				isSys = true
				break
			}
		}
		p := filepath.Join(stageDir, e.Name())
		if isSys {
			sysItems = append(sysItems, p)
		} else {
			remainingItems = append(remainingItems, p)
		}
	}
	newImage := mtoolsImageArg(newVHD, partStartSector*512)
	// IO.SYS/MSDOS.SYS/COMMAND.COM go through the same kernel loop-mount
	// + cp path as everything else, not raw mtools -- a real resize run
	// was observed to come back with a corrupted COMMAND.COM (partly
	// zeroed, wrong FAT chain) after a plain multi-file `mcopy` batch
	// write, even though the exact same batch succeeded cleanly in
	// isolated retries. Given the project's own hard-won lesson that
	// mtools' bulk-copy path can't be fully trusted for real file
	// content, system files get the reliable treatment too rather than
	// risk an intermittent repeat. They're still copied FIRST, in their
	// own loopCopyIn call, so they still claim early contiguous clusters
	// the way real DOS always does.
	if len(sysItems) > 0 {
		if err := loopCopyIn(newVHD, partStartSector*512, "", sysItems, false); err != nil {
			fatal("failed to write system files onto the new container: %v", err)
		}
	}
	if len(remainingItems) > 0 {
		if err := loopCopyIn(newVHD, partStartSector*512, "", remainingItems, true); err != nil {
			fatal("failed to copy files onto the new container: %v", err)
		}
	}

	applyAttrManifest(newImage, attrManifest, "")

	eprintln("Verifying copy...")
	verifyDir, err := os.MkdirTemp("", "resizevhd_verify_")
	if err != nil {
		fatal("%v", err)
	}
	defer os.RemoveAll(verifyDir)
	if err := mcopyOutTree(newImage, verifyDir); err != nil {
		fatal("verification read failed: %v", err)
	}
	if !treesMatch(stageDir, verifyDir) {
		fatal("verification failed -- copied content doesn't match the original.")
	}
	eprintln("Verified: all files match the original exactly.")

	eprintln()
	eprintln("=======================================================")
	eprintf(" Resize complete: %dMB -> %dMB\n", currentFileMB, sizeMB)
	eprintln("=======================================================")

	answer := promptLine(fmt.Sprintf("Replace the original file %s? [y/N]: ", sourceName))
	if answer == "y" || answer == "Y" {
		os.Remove(abs)
		os.Rename(newVHD, abs)
		cleanupNewVHD = false
		eprintf("Replaced. %s is now %dMB.\n", sourceName, sizeMB)
	} else {
		backupName := fmt.Sprintf("%s.bak-%s", abs, timestampNow())
		os.Rename(abs, backupName)
		os.Rename(newVHD, abs)
		cleanupNewVHD = false
		eprintf("Original kept as: %s\n", filepath.Base(backupName))
		eprintf("%s is now %dMB.\n", sourceName, sizeMB)
	}
	eprintln("=======================================================")
}

// treesMatch compares every regular file under a and b by relative
// path + md5, equivalent to the original's `find ... md5sum | sort`
// diff.
func treesMatch(a, b string) bool {
	ha, err1 := hashTree(a)
	hb, err2 := hashTree(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if len(ha) != len(hb) {
		return false
	}
	for k, v := range ha {
		if hb[k] != v {
			return false
		}
	}
	return true
}

func hashTree(root string) (map[string]string, error) {
	result := map[string]string{}
	var paths []string
	err := walkFiles(root, func(p string) {
		rel, rerr := filepath.Rel(root, p)
		if rerr == nil {
			paths = append(paths, rel)
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		f, err := os.Open(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		h := md5.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return nil, err
		}
		result[rel] = hex.EncodeToString(h.Sum(nil))
	}
	return result, nil
}

func timestampNow() string {
	return timeNowFormat("20060102150405")
}
