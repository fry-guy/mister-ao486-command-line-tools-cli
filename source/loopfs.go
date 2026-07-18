package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func findFreeLoop() (string, error) {
	out, err := runCapture("losetup", "-f")
	if err != nil {
		return "", fmt.Errorf("losetup -f: %v: %s", err, out)
	}
	return strings.TrimSpace(out), nil
}

func loopAttach(loop, file string, offsetBytes int64) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := runQuiet("losetup", "-o", strconv.FormatInt(offsetBytes, 10), loop, file); err == nil {
			return nil
		} else {
			lastErr = err
		}
		sleep(1)
	}
	cmd := exec.Command("losetup", "-o", strconv.FormatInt(offsetBytes, 10), loop, file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s (previous attempts: %v)", err, out, lastErr)
	}
	return nil
}

func loopDetach(loop string) {
	for attempt := 0; attempt < 5; attempt++ {
		if err := runQuiet("losetup", "-d", loop); err == nil {
			return
		}
		sleep(1)
	}
}

func loopCopyIn(vhd string, offsetBytes int64, destSubpath string, sources []string, showProgressHint bool) error {
	loop, err := findFreeLoop()
	if err != nil {
		return err
	}
	if err := loopAttach(loop, vhd, offsetBytes); err != nil {
		return fmt.Errorf("could not attach loop device for %s: %v", vhd, err)
	}
	defer loopDetach(loop)

	mnt, err := os.MkdirTemp("", "loopmnt_")
	if err != nil {
		return err
	}
	defer os.Remove(mnt)

	if err := runQuiet("mount", "-t", "vfat", "-o", "rw", loop, mnt); err != nil {
		return fmt.Errorf("could not mount %s for copying", vhd)
	}
	defer runQuiet("umount", mnt)

	target := mnt
	if destSubpath != "" {
		target = filepath.Join(mnt, destSubpath)
		if err := os.MkdirAll(target, 0755); err != nil {
			return err
		}
	}

	totalFiles := 0
	var totalBytes int64
	if showProgressHint {
		for _, src := range sources {
			if isDir(src) {
				f, b := dirStats(src)
				totalFiles += f
				totalBytes += b
			} else {
				totalFiles++
				if info, err := os.Stat(src); err == nil {
					totalBytes += info.Size()
				}
			}
		}
	}
	showProgress := showProgressHint && totalFiles >= 20
	baselineFiles := 0
	var baselineBytes int64
	if showProgress {
		baselineFiles, baselineBytes = dirStats(target)
	}

	for _, src := range sources {
		if showProgress {
			if err := cpRecursiveWithProgress(src, target, baselineFiles, totalFiles, baselineBytes, totalBytes); err != nil {
				fmt.Fprintln(os.Stderr)
				return err
			}
		} else {
			if err := cpRecursive(src, target); err != nil {
				return err
			}
		}
	}

	_ = runQuiet("sync")
	return nil
}

func cpRecursive(src, targetDir string) error {
	cmd := exec.Command("cp", "-r", src, targetDir+"/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp -r %s: %v: %s", src, err, out)
	}
	return nil
}

// dirStats returns the number of regular files and their combined
// size under root, recursively. Sampled periodically during a copy
// to drive live progress -- unlike a file's mere presence in a
// directory listing, its on-disk size grows as cp actually writes
// its content, so this reflects real progress even mid-way through
// one large file.
func dirStats(root string) (files int, bytes int64) {
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files++
			bytes += info.Size()
		}
		return nil
	})
	return
}

// cpRecursiveWithProgress runs `cp -r` in the background and prints
// a live progress line, sampled once a second, while it runs.
// Progress is tracked by bytes actually written rather than by file
// count: a directory holding one very large file among many small
// ones would otherwise hit "N/N files" almost immediately (each file
// only needs to exist in the listing, not be fully written, to count)
// and then appear to hang at 100% while that last file is still
// copying.
//
// loopCopyIn may call this once per top-level source item within a
// single overall copy (baselineFiles/totalFiles/baselineBytes/
// totalBytes are the SAME grand totals across every call in that
// batch), so an individual item finishing doesn't mean the whole
// batch is done. The progress line is only capped at 99% and only
// gets a trailing newline once the cumulative total genuinely
// reaches 100% -- i.e. on the last item's completion, not every
// item's.
func cpRecursiveWithProgress(src, targetDir string, baselineFiles, totalFiles int, baselineBytes, totalBytes int64) error {
	cmd := exec.Command("cp", "-r", src, targetDir+"/")
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	report := func(capAt99 bool) (copiedBytes, pct int64) {
		nowFiles, nowBytes := dirStats(targetDir)

		copiedFiles := nowFiles - baselineFiles
		if copiedFiles < 0 {
			copiedFiles = 0
		}
		if copiedFiles > totalFiles {
			copiedFiles = totalFiles
		}

		copiedBytes = nowBytes - baselineBytes
		if copiedBytes < 0 {
			copiedBytes = 0
		}
		if copiedBytes > totalBytes {
			copiedBytes = totalBytes
		}

		pct = 0
		if totalBytes > 0 {
			pct = copiedBytes * 100 / totalBytes
		}
		if capAt99 && pct > 99 {
			pct = 99
		}

		fmt.Fprintf(os.Stderr, "\r  Copying files: %d/%d, %s/%s (%d%%)  ",
			copiedFiles, totalFiles, humanSize(copiedBytes), humanSize(totalBytes), pct)
		return copiedBytes, pct
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			copiedBytes, _ := report(false) // real reading now that cp has actually exited
			if totalBytes == 0 || copiedBytes >= totalBytes {
				fmt.Fprintln(os.Stderr) // finalize the line only once the whole batch is done
			}
			return err
		case <-ticker.C:
			report(true) // capped at 99% while still actively copying
		}
	}
}
