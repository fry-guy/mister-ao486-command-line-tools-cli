package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// resolveMountTarget returns the file `mount vhd`/`mount chd`/`mount
// diskimage` should act on. Three forms are supported, matching how
// the rest of aotools already resolves ambiguous file arguments (see
// mglChoose/mglScanCandidates in mgl.go):
//
//   - An explicit file path (args[0], not a directory): used as-is.
//   - No argument at all: scans the current directory for files
//     matching exts.
//   - An explicit directory (args[0] is a directory, not a file):
//     scans that directory instead of the current one.
//
// In either scanning case, exactly one match is used automatically;
// multiple matches prompt the user to pick (mglChoose); zero matches
// is a clear, actionable error rather than a silent failure.
func resolveMountTarget(args []string, exts map[string]bool, label string) string {
	dir := "."
	if len(args) > 0 {
		if !isDir(args[0]) {
			return args[0]
		}
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal("%v", err)
	}
	candidates := scanExtCandidates(abs, exts)
	if len(candidates) == 0 {
		extList := make([]string, 0, len(exts))
		for e := range exts {
			extList = append(extList, e)
		}
		sort.Strings(extList)
		fatal("no %s file found in %s (looked for: %s). Run again with an explicit filename.",
			label, abs, strings.Join(extList, ", "))
	}
	return filepath.Join(abs, mglChoose(candidates, label))
}

// scanExtCandidates returns the sorted names of regular files
// directly under dir whose extension (case-insensitive) is in exts.
func scanExtCandidates(dir string, exts map[string]bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if exts[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// cmdMountVHD implements `aotools mount vhd <name.vhd>`. All
// human-facing narration goes to stderr; the resulting mountpoint
// path is printed alone as the LAST line of stdout, so a thin shell
// wrapper can do `cd "$(aotools mount vhd "$1")"` (a subprocess can
// never change its parent shell's own working directory, so the cd
// itself has to happen in the wrapper, not here).
func cmdMountVHD(args []string) {
	target := resolveMountTarget(args, map[string]bool{".vhd": true}, "VHD (.vhd)")
	vhd, err := filepath.Abs(target)
	if err != nil {
		fatal("%v", err)
	}
	if !fileExists(vhd) {
		fatal("file not found: %s", vhd)
	}

	startSector, err := partitionStartSector(vhd)
	if err != nil {
		fatal("could not determine partition offset: %v", err)
	}

	if err := os.MkdirAll(vhdMountPoint, 0755); err != nil {
		fatal("%v", err)
	}
	offset := startSector * 512
	if err := runQuiet("mount", "-o", fmt.Sprintf("loop,offset=%d", offset), vhd, vhdMountPoint); err != nil {
		fatal("failed to mount %s", vhd)
	}
	eprintf("Mounted %s at %s (offset=%d)\n", vhd, vhdMountPoint, offset)
	eprintln("Run 'umountvhd' when done to unmount and clean up.")
	fmt.Println(vhdMountPoint)
}

// cmdUmountVHD implements `aotools umount vhd`.
func cmdUmountVHD() {
	if isMounted(vhdMountPoint) {
		if err := runQuiet("umount", vhdMountPoint); err != nil {
			fatal("failed to unmount %s", vhdMountPoint)
		}
		eprintf("Unmounted %s.\n", vhdMountPoint)
	} else {
		eprintf("Nothing mounted at %s.\n", vhdMountPoint)
	}
	_ = os.Remove(vhdMountPoint)
}

// cmdMountIMA implements `aotools mount diskimage <name.ima>`. Unlike
// VHDs, .ima floppy images are raw "superfloppy" images with no
// partition table (cmdIMACreate formats them directly with
// `mkfs.vfat -I`), so this is a plain loop mount with no partition
// offset needed.
func cmdMountIMA(args []string) {
	target := resolveMountTarget(args, map[string]bool{".ima": true, ".img": true}, "disk image (.ima/.img)")
	ima, err := filepath.Abs(target)
	if err != nil {
		fatal("%v", err)
	}
	if !fileExists(ima) {
		fatal("file not found: %s", ima)
	}
	if isMounted(imaMountPoint) {
		fatal("%s is already mounted. Run 'aotools umount diskimage' first.", imaMountPoint)
	}

	if err := os.MkdirAll(imaMountPoint, 0755); err != nil {
		fatal("%v", err)
	}
	if err := runQuiet("mount", "-o", "loop", ima, imaMountPoint); err != nil {
		fatal("failed to mount %s", ima)
	}
	eprintf("Mounted %s at %s\n", ima, imaMountPoint)
	eprintln("Run 'aotools umount diskimage' when done to unmount and clean up.")
	fmt.Println(imaMountPoint)
}

// cmdUmountIMA implements `aotools umount diskimage`.
func cmdUmountIMA() {
	if isMounted(imaMountPoint) {
		if err := runQuiet("umount", imaMountPoint); err != nil {
			fatal("failed to unmount %s", imaMountPoint)
		}
		eprintf("Unmounted %s.\n", imaMountPoint)
	} else {
		eprintf("Nothing mounted at %s.\n", imaMountPoint)
	}
	_ = os.Remove(imaMountPoint)
}

// cmdMountCHD implements `aotools mount chd <name.chd>`, extracting
// the disc with chdman, detecting the sector format, stripping raw-
// sector wrappers if needed to produce a mountable ISO, then mounting
// it read-only. The extracted copy lives on /media/fat (not /tmp)
// since large discs could exhaust RAM-backed scratch space.
func cmdMountCHD(args []string) {
	target := resolveMountTarget(args, map[string]bool{".chd": true}, "CHD (.chd)")
	if !fileExists(target) {
		fatal("file not found: %s", target)
	}
	if !fileExists(chdmanBin) {
		fatal("chdman not found or not executable at %s", chdmanBin)
	}
	if isMounted(chdMountPoint) {
		fatal("%s is already mounted. Run 'umountchd' first.", chdMountPoint)
	}
	chd, err := filepath.Abs(target)
	if err != nil {
		fatal("%v", err)
	}

	os.RemoveAll(chdExtractDir)
	if err := os.MkdirAll(chdExtractDir, 0755); err != nil {
		fatal("%v", err)
	}

	info, _ := runCapture(chdmanBin, "info", "-i", chd)
	if !strings.Contains(info, "Metadata:") {
		os.RemoveAll(chdExtractDir)
		fatal("%s doesn't look like a CD image CHD (no track metadata).\nmountchd only supports CHDs created by mkchd (createcd).", chd)
	}

	eprintln("Extracting " + chd + " (this can take a while for larger discs)...")
	cueOut := filepath.Join(chdExtractDir, "track.cue")
	binOut := filepath.Join(chdExtractDir, "track.bin")
	cmd := exec.Command(chdmanBin, "extractcd", "-i", chd, "-o", cueOut, "-ob", binOut)
	cmd.Stdin = nil
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(chdExtractDir)
		fatal("chdman failed to extract %s: %v", chd, err)
	}

	trackMode := readCueTrackMode(cueOut)
	var isoPath string
	switch trackMode {
	case "MODE1/2352", "MODE2/2352":
		eprintln("Raw 2352-byte sectors detected; extracting user data...")
		isoPath = filepath.Join(chdExtractDir, "track.iso")
		if err := stripRawSectors(binOut, isoPath); err != nil {
			os.RemoveAll(chdExtractDir)
			fatal("%v", err)
		}
		os.Remove(binOut)
	case "MODE1/2048":
		isoPath = filepath.Join(chdExtractDir, "track.iso")
		if err := os.Rename(binOut, isoPath); err != nil {
			os.RemoveAll(chdExtractDir)
			fatal("%v", err)
		}
	default:
		os.RemoveAll(chdExtractDir)
		fatal("unsupported track mode '%s' -- only data tracks (MODE1/MODE2) are mountable as a filesystem.", trackMode)
	}

	if err := os.MkdirAll(chdMountPoint, 0755); err != nil {
		fatal("%v", err)
	}
	if err := runQuiet("mount", "-o", "loop,ro", isoPath, chdMountPoint); err != nil {
		os.Remove(chdMountPoint)
		os.RemoveAll(chdExtractDir)
		fatal("failed to mount extracted image.")
	}

	eprintf("Mounted %s at %s\n", chd, chdMountPoint)
	eprintln("Run 'umountchd' when done to unmount and clean up.")
	fmt.Println(chdMountPoint)
}

// cmdUmountCHD implements `aotools umount chd`.
func cmdUmountCHD() {
	if isMounted(chdMountPoint) {
		_ = runQuiet("umount", chdMountPoint)
	}
	_ = os.Remove(chdMountPoint)
	_ = os.RemoveAll(chdExtractDir)
	eprintln("Unmounted and cleaned up.")
}

// cmdUmountAuto implements bare `aotools umount` with no vhd/chd/
// diskimage argument: it figures out which one you're actually in
// and unmounts that, instead of making you specify it. This exists
// mainly for direct-binary invocation (full path, no shell
// integration loaded) -- the shell wrapper (shellinit.go) resolves
// the type from $PWD itself before ever reaching here, since only it
// can cd the parent shell back to where it was; calling the binary
// directly bypasses that and lands in this function instead.
//
// Detection order: current directory first (matches whichever mount
// you're actually sitting inside, mirroring the shell wrapper's own
// logic), then -- if cwd doesn't tell us -- whichever single type is
// actually mounted. If more than one type is mounted and cwd doesn't
// disambiguate, this refuses to guess and asks for an explicit type.
func cmdUmountAuto() {
	if wd, err := os.Getwd(); err == nil {
		switch {
		case wd == vhdMountPoint || strings.HasPrefix(wd, vhdMountPoint+"/"):
			cmdUmountVHD()
			return
		case wd == chdMountPoint || strings.HasPrefix(wd, chdMountPoint+"/"):
			cmdUmountCHD()
			return
		case wd == imaMountPoint || strings.HasPrefix(wd, imaMountPoint+"/"):
			cmdUmountIMA()
			return
		}
	}

	var mounted []string
	if isMounted(vhdMountPoint) {
		mounted = append(mounted, "vhd")
	}
	if isMounted(chdMountPoint) {
		mounted = append(mounted, "chd")
	}
	if isMounted(imaMountPoint) {
		mounted = append(mounted, "diskimage")
	}

	switch len(mounted) {
	case 0:
		fatal("nothing is currently mounted (vhd, chd, or diskimage)")
	case 1:
		switch mounted[0] {
		case "vhd":
			cmdUmountVHD()
		case "chd":
			cmdUmountCHD()
		case "diskimage":
			cmdUmountIMA()
		}
	default:
		fatal("multiple types are mounted (%s) and the current directory doesn't indicate which -- run 'aotools umount <vhd|chd|diskimage>' explicitly", strings.Join(mounted, ", "))
	}
}

func isMounted(path string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		// Fall back to `mount` if /proc isn't available for some reason.
		out, _ := runCapture("mount")
		return strings.Contains(out, " "+path+" ")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == path {
			return true
		}
	}
	return false
}

func readCueTrackMode(cuePath string) string {
	f, err := os.Open(cuePath)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[0] == "TRACK" {
			return fields[2]
		}
	}
	return ""
}

// stripRawSectors extracts the 2048-byte user-data portion (at a
// fixed 16-byte offset) from every raw 2352-byte CD sector, producing
// a plain mountable ISO. Mirrors the inline python one-liner in
// mountchd.
func stripRawSectors(binPath, isoPath string) error {
	const sectorSize = 2352
	const dataOffset = 16
	const dataLength = 2048

	inFile, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer inFile.Close()
	outFile, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Buffered on both sides: a raw 2352-byte read + 2048-byte write per
	// sector means ~350,000 syscall pairs for a typical CD image, which
	// is slow enough on ARM to be noticeable (confirmed: ~1MB/s
	// unbuffered vs. tens of MB/s buffered on real MiSTer hardware).
	in := bufio.NewReaderSize(inFile, 1<<20)
	out := bufio.NewWriterSize(outFile, 1<<20)
	defer out.Flush()

	buf := make([]byte, sectorSize)
	for {
		n, rerr := io.ReadFull(in, buf)
		if n < sectorSize {
			break
		}
		if _, werr := out.Write(buf[dataOffset : dataOffset+dataLength]); werr != nil {
			return werr
		}
		if rerr != nil {
			break
		}
	}
	return out.Flush()
}
