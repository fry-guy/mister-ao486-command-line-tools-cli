package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// eprintf writes a formatted status/info line to stderr. All the
// human-facing "Copying files...", "Done: X" style narration goes to
// stderr; stdout is reserved for machine-readable output (currently
// only used by `mount vhd`/`mount chd`, which print the resulting
// mountpoint as the last line of stdout so a thin shell wrapper can
// `cd "$(aotools mount vhd ...)"`).
func eprintf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
}

func eprintln(a ...interface{}) {
	fmt.Fprintln(os.Stderr, a...)
}

// fatalExit is the panic value fatal() uses to unwind the call stack
// -- running every registered defer along the way, including scratch
// staging-directory cleanup (os.RemoveAll(stageDir) etc.) -- before
// the process actually exits. fatal() previously called os.Exit(1)
// directly, which terminates the process immediately and skips ALL
// deferred cleanup; a failed resize vhd/create vhd -win31/create
// diskimage partway through could leave its staging directory behind
// under the big-scratch area (documented as a "Known limitation" in
// NOTES.md, then directly observed to cause a real failure elsewhere
// once enough orphaned directories piled up). main() recovers this
// specific type and exits with status 1 only after every defer along
// the failing call stack has already run; any other panic value is
// re-panicked so a genuine bug still surfaces as a real crash instead
// of being silently swallowed.
type fatalExit struct{}

func fatal(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
	panic(fatalExit{})
}

// runCapture runs a command and returns combined stdout+stderr, trimmed.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// runLive runs a command with stdout/stderr wired to ours (for things
// like mkfs.vfat we still want to capture, so this is only used where
// we genuinely want passthrough).
func runLive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}

// runQuiet runs a command, discarding output, returning error only.
func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = nil
	_, err := cmd.CombinedOutput()
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// stdinReader is shared across every promptLine call. A fresh
// bufio.Reader per call (the original approach) discards whatever
// it had already buffered-but-unread from the fd the moment the
// function returns -- harmless with a real interactive terminal
// delivering one line at a time, but a real bug against piped/
// scripted input: a single Read() can pull in several queued lines
// at once, and every line after the first one is silently lost the
// next time promptLine is called. Caught by testing the new
// install-time download flow (two sequential prompts) with piped
// input: the second prompt kept coming back empty even though the
// answer was clearly there in the pipe.
var stdinReader = bufio.NewReader(os.Stdin)

func promptLine(prompt string) string {
	eprintf("%s", prompt)
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// ensureMtoolsSymlinks recreates the mcopy/mattrib -> mtools symlinks
// mtools itself relies on to know which sub-command it's acting as
// (mtools dispatches purely on argv[0]). Mirrors the auto-create-on-
// first-use behavior every original script had.
func ensureMtoolsSymlinks() error {
	if !fileExists(mtoolsBin) {
		return fmt.Errorf("%s not found or not executable", mtoolsBin)
	}
	if !fileExists(mcopyBin) {
		_ = os.Symlink("mtools", mcopyBin)
	}
	if !fileExists(mattribBin) {
		_ = os.Symlink("mtools", mattribBin)
	}
	return nil
}

// mktempBig creates a scratch staging directory under bigScratchBase
// (on /media/fat, not the RAM-backed /tmp) for archive-scale staging.
// /tmp on MiSTer is tmpfs with only ~240MB, which large game archives
// (Windows 3.1 titles especially) can exhaust mid-copy.
func mktempBig() (string, error) {
	if err := os.MkdirAll(bigScratchBase, 0755); err != nil {
		return "", err
	}
	return os.MkdirTemp(bigScratchBase, "stage_")
}

func sweepBigScratch() {
	_ = os.RemoveAll(bigScratchBase)
}

// sizeMB rounds bytes up to whole megabytes.
func mbCeil(bytes int64) int64 {
	return (bytes + 1048575) / 1048576
}

func kbCeil(bytes int64) int64 {
	return (bytes + 1023) / 1024
}

// countFiles counts regular files under root (recursively).
func countFiles(root string) int {
	n := 0
	_ = walkFiles(root, func(string) { n++ })
	return n
}

func walkFiles(root string, fn func(path string)) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := root + "/" + e.Name()
		if e.IsDir() {
			_ = walkFiles(p, fn)
		} else {
			fn(p)
		}
	}
	return nil
}

// copyDirTree recursively copies every file and subdirectory from src
// into dst (creating dst and any needed subdirectories), preserving
// relative structure. Used when a create vhd/mgl/diskimage source is
// a plain folder rather than an archive -- extractArchive only knows
// how to unpack .zip/.tar/etc, so folder sources are copied directly
// instead of routed through it.
func copyDirTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func sleep(seconds float64) {
	time.Sleep(time.Duration(seconds * float64(time.Second)))
}

func timeNowFormat(layout string) string {
	return time.Now().Format(layout)
}

// createSparseFile creates (or truncates) path to exactly size bytes,
// equivalent to `truncate -s <size>`. On most Linux filesystems this
// is sparse (no actual disk writes for the zero-filled region) until
// something -- mkfs.vfat, a loop-mounted copy, etc -- writes into it.
func createSparseFile(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Truncate(size)
}
