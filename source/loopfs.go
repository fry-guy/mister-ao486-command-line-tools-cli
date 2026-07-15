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

// findFreeLoop returns the next free loop device, as `losetup -f`.
func findFreeLoop() (string, error) {
	out, err := runCapture("losetup", "-f")
	if err != nil {
		return "", fmt.Errorf("losetup -f: %v: %s", err, out)
	}
	return strings.TrimSpace(out), nil
}

// loopAttach attaches file to loop at the given byte offset, retrying
// up to 5 times with a 1s pause: losetup can transiently fail with
// "Resource temporarily unavailable" right after a previous loop
// device (especially one whose backing file had a partition table,
// triggering a kernel auto partition-scan of a loopNpM sub-device) is
// torn down. Mirrors loop_attach().
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
	// Final attempt without suppressing the error, so a genuine
	// failure still surfaces a real message.
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

// loopCopyIn mounts vhd (loop, offset offsetBytes) read-write, copies
// sources into mnt/destSubpath (or mnt root if destSubpath is empty)
// via `cp -r`, then unmounts. Used for any bulk copy of real archive
// content: mtools' recursive mcopy -s was found to silently drop
// files/subdirectories on real, large archives, confirmed directly;
// the kernel loop-mount + cp path gave byte-for-byte perfect results
// every time. Needs root, same as mkfs.vfat already does. Mirrors
// loop_copy_in().
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

	// Count files up front so large copies (archive injections can be
	// 1000+ files, a couple of minutes) show progress instead of
	// sitting silent.
	totalFiles := 0
	if showProgressHint {
		for _, src := range sources {
			if isDir(src) {
				totalFiles += countFiles(src)
			} else {
				totalFiles++
			}
		}
	}
	showProgress := showProgressHint && totalFiles >= 20
	baseline := 0
	if showProgress {
		baseline = countFiles(target)
	}

	for _, src := range sources {
		if showProgress {
			if err := cpRecursiveWithProgress(src, target, baseline, totalFiles); err != nil {
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

func cpRecursiveWithProgress(src, targetDir string, baseline, totalFiles int) error {
	cmd := exec.Command("cp", "-r", src, targetDir+"/")
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			pct := 100
			fmt.Fprintf(os.Stderr, "\r  Copying files: %d/%d (%d%%)      \n", totalFiles, totalFiles, pct)
			return err
		case <-ticker.C:
			now := countFiles(targetDir)
			copied := now - baseline
			if copied < 0 {
				copied = 0
			}
			pct := 0
			if totalFiles > 0 {
				pct = copied * 100 / totalFiles
			}
			if pct > 100 {
				pct = 100
			}
			fmt.Fprintf(os.Stderr, "\r  Copying files: %d/%d (%d%%)  ", copied, totalFiles, pct)
		}
	}
}
