package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// clusterSectorsFor returns the mkfs.vfat cluster size (in 512-byte
// sectors) and whether FAT16 must be forced (-F 16), for a given VHD
// size. Below 16MB we deliberately let mkfs.vfat pick FAT12 vs FAT16
// naturally from the resulting cluster count; forcing -F16 there
// caused "General failure reading drive C" on small VHDs. At 16MB and
// above mkfs.vfat's own heuristic silently switches to FAT32 well
// before the largest supported size unless forced, and MS-DOS 6.22
// cannot read FAT32 at all. Brackets verified empirically against
// real mkfs.vfat/DOS behavior.
func clusterSectorsFor(sizeMB int64) (sectors int, forceFat16 bool) {
	switch {
	case sizeMB < 16:
		return 1, false
	case sizeMB <= 128:
		return 4, true
	case sizeMB <= 256:
		return 8, true
	case sizeMB <= 512:
		return 16, true
	case sizeMB <= 1024:
		return 32, true
	default:
		return 64, true
	}
}

// formatAndFixBPB writes the MBR+partition table, formats the FAT
// volume with the size-appropriate cluster size, and fixes up the
// BPB/partition-type byte for MS-DOS compatibility. outPath must
// already exist at its final size (truncated).
func formatAndFixBPB(outPath string, sizeMB int64) error {
	if err := makeMBR(outPath, sizeMB); err != nil {
		return err
	}

	sectors, forceFat16 := clusterSectorsFor(sizeMB)
	loop, err := findFreeLoop()
	if err != nil {
		return err
	}
	if err := loopAttach(loop, outPath, partStartSector*512); err != nil {
		return err
	}
	args := []string{"-I"}
	if forceFat16 {
		args = append(args, "-F", "16")
	}
	args = append(args, "-s", strconv.Itoa(sectors), loop)
	out, mkfsErr := runCapture("mkfs.vfat", args...)
	loopDetach(loop)
	if mkfsErr != nil {
		return fmt.Errorf("mkfs.vfat failed:\n%s", out)
	}

	return fixBPB(outPath, partStartSector, sizeMB)
}

// dosOverheadBytes dynamically measures the fixed MS-DOS 6.22 overhead
// every -dos VHD pays: the DOS/MISTER/DRIVERS/UTIL folders (from
// dos_template.vhd, measured directly so this self-corrects if those
// folders ever change) plus a fixed 195,215-byte allowance for the
// FORMAT /S system files and FAT/root-dir overhead that isn't visible
// as regular files inside the template. Mirrors dos_overhead_bytes().
func dosOverheadBytes() int64 {
	const floor = 500000
	const fallback = 767857
	var folderBytes int64

	for attempt := 0; attempt < 3; attempt++ {
		loop, err := findFreeLoop()
		if err == nil {
			if loopAttach(loop, dosTemplate, partStartSector*512) == nil {
				mnt, merr := os.MkdirTemp("", "dosoverhead_")
				if merr == nil {
					if runQuiet("mount", "-t", "vfat", "-o", "ro", loop, mnt) == nil {
						folderBytes = dirSizeBytes(filepath.Join(mnt, "DOS")) +
							dirSizeBytes(filepath.Join(mnt, "MISTER")) +
							dirSizeBytes(filepath.Join(mnt, "DRIVERS")) +
							dirSizeBytes(filepath.Join(mnt, "UTIL"))
						_ = runQuiet("umount", mnt)
					}
					_ = os.Remove(mnt)
				}
			}
			loopDetach(loop)
		}
		if folderBytes >= floor {
			break
		}
		folderBytes = 0
		sleep(1)
	}
	if folderBytes < floor {
		folderBytes = fallback
	}
	return folderBytes + 195215
}

// prepareAutomationFloppy builds the boot floppy used to drive the
// real-DOS FORMAT+XCOPY sequence under QEMU: a copy of disk1.img with
// the embedded AUTOEXEC.BAT/CONFIG.SYS written on via mtools (no root
// needed -- floppy images have no partition table).
func prepareAutomationFloppy() error {
	if err := copyFile(floppySrc, helperImg); err != nil {
		return err
	}
	autoexecTmp, err := os.CreateTemp("", "autoexec_")
	if err != nil {
		return err
	}
	defer os.Remove(autoexecTmp.Name())
	if _, err := autoexecTmp.WriteString(qemuAutoexecBat()); err != nil {
		autoexecTmp.Close()
		return err
	}
	autoexecTmp.Close()
	if err := mcopyPut(helperImg, []string{autoexecTmp.Name()}, "AUTOEXEC.BAT"); err != nil {
		return err
	}

	configTmp, err := os.CreateTemp("", "config_")
	if err != nil {
		return err
	}
	defer os.Remove(configTmp.Name())
	if _, err := configTmp.WriteString("FILES=30\r\nBUFFERS=20\r\nLASTDRIVE=Z\r\n"); err != nil {
		configTmp.Close()
		return err
	}
	configTmp.Close()
	return mcopyPut(helperImg, []string{configTmp.Name()}, "CONFIG.SYS")
}

// cmdVHDCreate implements `aotools create vhd [-dos|-win31]
// <name.vhd> [archive|folder]`, porting mkvhd in full.
func cmdVHDCreate(args []string) {
	newgame := false
	win31 := false
	if len(args) > 0 && args[0] == "-dos" {
		newgame = true
		args = args[1:]
	} else if len(args) > 0 && args[0] == "-win31" {
		win31 = true
		args = args[1:]
	}

	if len(args) == 0 {
		eprintln("Usage:")
		eprintln("  aotools create vhd <name.vhd>                          create a blank VHD")
		eprintln("  aotools create vhd -dos <name.vhd>                     create + auto-format with DOS")
		eprintln("  aotools create vhd -dos <name.vhd> <archive|folder>    also inject <archive|folder>")
		eprintln("  aotools create vhd -win31 <name.vhd>                   create + install Windows 3.1")
		eprintln("  aotools create vhd -win31 <name.vhd> <archive|folder>  also inject <archive|folder>")
		os.Exit(1)
	}

	outName := args[0]
	var archive string
	if len(args) > 1 {
		archive = args[1]
	}
	if !strings.HasSuffix(strings.ToLower(outName), ".vhd") {
		outName += ".vhd"
	}
	destDir, _ := os.Getwd()
	outPath := filepath.Join(destDir, outName)

	if fileExists(outPath) {
		fatal("%s already exists.", outPath)
	}

	defer sweepBigScratch()

	if win31 {
		createWin31VHD(outPath, archive)
		return
	}

	if archive != "" {
		if !newgame {
			fatal("an archive argument requires -dos.")
		}
		if !fileExists(archive) {
			fatal("archive or folder not found: %s", archive)
		}
		if !isDir(archive) && !isSupportedArchive(archive) {
			fatal("unsupported source: %s\nSupported: .zip, .tar, .tar.gz/.tgz, .tar.bz2/.tbz2, or a folder", archive)
		}
	}

	var suggestedMB int64
	var archiveBytes int64
	if archive != "" {
		eprintln("Calculating suggested VHD size...")
		if isDir(archive) {
			archiveBytes = dirSizeBytes(archive)
		} else {
			archiveBytes = archiveUncompressedSize(archive)
		}
		dosBytes := dosOverheadBytes()
		headroom := archiveBytes / 4
		if headroom < 102400 {
			headroom = 102400
		}
		total := archiveBytes + dosBytes + headroom
		suggestedMB = mbCeil(total)
		eprintf("  Archive content: %d KB\n", kbCeil(archiveBytes))
		eprintf("  Suggested size:  %d MB\n", suggestedMB)
		eprintln()
	}

	sizeMBStr := promptLine("Enter VHD size in MB: ")
	if sizeMBStr == "" && suggestedMB > 0 {
		sizeMBStr = strconv.FormatInt(suggestedMB, 10)
	}
	sizeMB, err := strconv.ParseInt(sizeMBStr, 10, 64)
	if err != nil {
		fatal("size must be a whole number.")
	}

	if newgame && sizeMB < 2 {
		eprintln("Error: VHD size too small for -dos (auto-DOS-setup).")
		eprintln("MS-DOS's own system files need ~1MB by themselves; there's no")
		eprintln("room left for any game data below ~2MB. Use at least 2MB, or")
		eprintln("drop -dos to create a blank VHD of any size and format it yourself.")
		os.Exit(1)
	}
	if sizeMB >= 2048 {
		eprintln("Error: size exceeds the real MS-DOS FAT16 limit.")
		eprintln("FAT16 tops out at 2047MB even with the largest standard")
		eprintln("(32KB) cluster size; MS-DOS 6.22 cannot read FAT32. Use at")
		eprintln("most 2047MB, or split the game across multiple VHDs.")
		os.Exit(1)
	}

	eprintln()
	eprintf("Creating blank %dMB VHD...\n", sizeMB)
	if err := createSparseFile(outPath, sizeMB*1024*1024); err != nil {
		fatal("%v", err)
	}

	if !newgame {
		eprintln()
		eprintf("Done. Blank VHD: %s\n", outPath)
		return
	}

	for _, req := range []string{floppySrc, dosTemplate, qemuBin} {
		if !fileExists(req) {
			os.Remove(outPath)
			fatal("required file not found: %s", req)
		}
	}
	if !isDir(qemuBiosDir) {
		os.Remove(outPath)
		fatal("%s not found", qemuBiosDir)
	}
	if err := ensureMtoolsSymlinks(); err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}

	eprintln("Writing partition table...")
	if err := makeMBR(outPath, sizeMB); err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}
	eprintln("Formatting filesystem...")
	sectors, forceFat16 := clusterSectorsFor(sizeMB)
	loop, err := findFreeLoop()
	if err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}
	if err := loopAttach(loop, outPath, partStartSector*512); err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}
	mkfsArgs := []string{"-I"}
	if forceFat16 {
		mkfsArgs = append(mkfsArgs, "-F", "16")
	}
	mkfsArgs = append(mkfsArgs, "-s", strconv.Itoa(sectors), loop)
	mkfsOut, mkfsErr := runCapture("mkfs.vfat", mkfsArgs...)
	loopDetach(loop)
	if mkfsErr != nil {
		os.Remove(outPath)
		fatal("mkfs.vfat failed to format %dMB VHD:\n%s", sizeMB, mkfsOut)
	}

	eprintln("Fixing BPB for MS-DOS compatibility...")
	if err := fixBPB(outPath, partStartSector, sizeMB); err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}

	eprintln("Preparing automation floppy...")
	if err := prepareAutomationFloppy(); err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}

	eprintln()
	eprintln("Running QEMU to format and populate VHD...")
	eprintln("This will take 1-2 minutes...")
	eprintln()

	if !runDOSInstall(outPath, helperImg, dosTemplate) {
		eprintf("[!] Setup may have failed. Try: aotools mount vhd %s\n", outPath)
		os.Exit(1)
	}

	eprintln()
	eprintln("=======================================================")
	eprintf(" Done! VHD ready: %s\n", outPath)
	eprintln("=======================================================")

	if archive != "" {
		injectDOSArchive(outPath, archive)
	}

	eprintln("=======================================================")
	eprintf(" Mount: aotools mount vhd %s\n", outPath)
	eprintln("=======================================================")
}

// injectDOSArchive copies archive (an archive file, or a plain
// folder) into its own game folder on outPath (already a working
// -dos VHD), lets the user pick a launch executable, and appends the
// CD/launch lines to AUTOEXEC.BAT. Mirrors the archive-injection
// block at the end of mkvhd -dos.
func injectDOSArchive(outPath, archive string) {
	gameName := dosShortName(archive)
	eprintln()
	eprintf("Injecting %s into \\%s ...\n", archive, gameName)

	stageDir, err := mktempBig()
	if err != nil {
		eprintf("Error: %v\n", err)
		return
	}
	defer os.RemoveAll(stageDir)
	gameDir := filepath.Join(stageDir, gameName)
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		eprintf("Error: %v\n", err)
		return
	}
	if isDir(archive) {
		if err := copyDirTree(archive, gameDir); err != nil {
			eprintf("Error: %v\n", err)
			return
		}
	} else if err := extractArchive(archive, gameDir); err != nil {
		eprintf("Error: %v\n", err)
		return
	}
	if err := flattenWrapperDirs(gameDir); err != nil {
		eprintf("Error: %v\n", err)
	}
	launchExe, launchSubdir, err := selectLaunchExecutable(gameDir)
	if err != nil {
		eprintf("Error: %v\n", err)
	}

	var attrManifest map[string]byte
	if classifyArchive(archive) == archZip {
		attrManifest, _ = zipAttrManifest(archive)
	}

	if err := loopCopyIn(outPath, partStartSector*512, gameName, []string{gameDir + "/."}, true); err != nil {
		eprintf("Error: failed to copy %s onto the VHD: %v\n", archive, err)
		return
	}
	image := mtoolsImageArg(outPath, partStartSector*512)
	applyAttrManifest(image, attrManifest, gameName)

	autoexecTmp, err := os.CreateTemp("", "autoexec_upd_")
	if err == nil {
		defer os.Remove(autoexecTmp.Name())
		autoexecTmp.Close()
		_ = mcopyOut(image, "AUTOEXEC.BAT", autoexecTmp.Name())
		f, ferr := os.OpenFile(autoexecTmp.Name(), os.O_APPEND|os.O_WRONLY, 0644)
		if ferr == nil {
			fmt.Fprintf(f, "CD %s\r\n", gameName)
			if launchSubdir != "" {
				fmt.Fprintf(f, "CD %s\r\n", launchSubdir)
			}
			if launchExe != "" {
				fmt.Fprintf(f, "%s\r\n", launchExe)
			}
			f.Close()
			_ = mcopyPut(image, []string{autoexecTmp.Name()}, "AUTOEXEC.BAT")
		}
	}

	eprintf("Game files extracted to \\%s\n", gameName)
	eprintf("AUTOEXEC.BAT updated with: CD %s\n", gameName)
	if launchSubdir != "" {
		eprintf("                           CD %s\n", launchSubdir)
	}
	if launchExe != "" {
		eprintf("                           %s\n", launchExe)
	}

	promptDeleteSource(archive)
}

// createWin31VHD implements the -win31 branch of mkvhd: clones
// win31_template.vhd (a complete, already-working DOS+Windows 3.1+
// ao486-driver install with no game on it) onto a fresh container,
// preserving the template's own boot sector (paired with its own
// system files) rather than writing a fresh one, then optionally
// injects a game archive and points WIN.INI's run= line at it.
func createWin31VHD(outPath, archive string) {
	if !fileExists(win31Template) {
		fatal("required file not found: %s", win31Template)
	}
	if err := ensureMtoolsSymlinks(); err != nil {
		fatal("%v", err)
	}
	if archive != "" {
		if !fileExists(archive) {
			fatal("archive or folder not found: %s", archive)
		}
		if !isDir(archive) && !isSupportedArchive(archive) {
			fatal("unsupported source: %s\nSupported: .zip, .tar, .tar.gz/.tgz, .tar.bz2/.tbz2, or a folder", archive)
		}
	}

	templateImage := mtoolsImageArg(win31Template, partStartSector*512)
	stageDir, err := mktempBig()
	if err != nil {
		fatal("%v", err)
	}
	defer os.RemoveAll(stageDir)
	if err := mcopyOutTree(templateImage, stageDir); err != nil {
		fatal("failed to read %s: %v", win31Template, err)
	}

	bootOrig, err := readBootSector(win31Template, partStartSector)
	if err != nil {
		fatal("%v", err)
	}
	attrManifest, err := readFatAttrManifest(win31Template, partStartSector)
	if err != nil {
		attrManifest = map[string]byte{}
	}

	templateBytes := dirSizeBytes(stageDir)
	var archiveBytes int64
	if archive != "" {
		if isDir(archive) {
			archiveBytes = dirSizeBytes(archive)
		} else {
			archiveBytes = archiveUncompressedSize(archive)
		}
	}
	actualBytes := templateBytes + archiveBytes
	headroom := actualBytes / 4
	if headroom < 1048576 {
		headroom = 1048576
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
	if archive != "" {
		eprintf("  Windows 3.1 + archive content: %d KB\n", kbCeil(actualBytes))
	} else {
		eprintf("  Windows 3.1 content: %d KB\n", kbCeil(actualBytes))
	}
	eprintf("  Suggested VHD size:  %d MB\n", suggestedMB)
	eprintln()

	sizeMBStr := promptLine(fmt.Sprintf("Enter VHD size in MB [%d]: ", suggestedMB))
	sizeMB := suggestedMB
	if sizeMBStr != "" {
		v, err := strconv.ParseInt(sizeMBStr, 10, 64)
		if err != nil {
			fatal("size must be a whole number.")
		}
		sizeMB = v
	}
	if sizeMB < 2 || sizeMB >= 2048 {
		fatal("size must be between 2 and 2047 MB.")
	}
	neededMB := mbCeil(templateBytes)
	if sizeMB < neededMB {
		fatal("%dMB is too small to hold Windows 3.1 (~%dMB).", sizeMB, neededMB)
	}

	eprintln()
	eprintf("Creating %dMB container...\n", sizeMB)
	if err := createSparseFile(outPath, sizeMB*1024*1024); err != nil {
		fatal("%v", err)
	}

	eprintln("Formatting filesystem...")
	if err := formatAndFixBPB(outPath, sizeMB); err != nil {
		os.Remove(outPath)
		fatal("%v", err)
	}

	eprintln("Copying Windows 3.1 files...")
	sysFiles := []string{"IO.SYS", "MSDOS.SYS", "COMMAND.COM"}
	sysSet := map[string]bool{}
	for _, sf := range sysFiles {
		sysSet[strings.ToUpper(sf)] = true
	}
	entries, _ := os.ReadDir(stageDir)
	// sysItems is built by iterating sysFiles (fixed IO.SYS, MSDOS.SYS,
	// COMMAND.COM order), NOT by iterating entries -- os.ReadDir returns
	// entries sorted alphabetically. See the identical fix (and its full
	// explanation) in cmdVHDResize/vhdresize.go: MS-DOS 6.22's boot
	// sector loads system files by directory-entry POSITION, not by
	// name, so building this list in alphabetical order put COMMAND.COM
	// in the first root directory slot instead of IO.SYS, breaking boot
	// with "Non-System disk or disk error" despite every file's content
	// being correct. This exact bug was found and fixed in resize vhd
	// first; createWin31VHD has the identical pattern and needed the
	// identical fix.
	var sysItems []string
	for _, sf := range sysFiles {
		for _, e := range entries {
			if strings.EqualFold(e.Name(), sf) {
				sysItems = append(sysItems, filepath.Join(stageDir, e.Name()))
				break
			}
		}
	}
	var remainingItems []string
	for _, e := range entries {
		if !sysSet[strings.ToUpper(e.Name())] {
			remainingItems = append(remainingItems, filepath.Join(stageDir, e.Name()))
		}
	}
	// sysItems and remainingItems are copied via ONE combined loopCopyIn
	// call, not two separate ones -- see the identical fix in
	// cmdVHDResize/vhdresize.go. Two separate loop-attach/mount cycles on
	// this hardware's kernel/loop-driver combination were observed (on
	// resize vhd, same underlying mechanism) to let the second mount see
	// a stale, pre-first-call view of the root directory and overwrite
	// the first call's directory entries as if their slots were still
	// free. A single mount session removes that window entirely.
	mainItems := append(append([]string{}, sysItems...), remainingItems...)
	if len(mainItems) > 0 {
		if err := loopCopyIn(outPath, partStartSector*512, "", mainItems, true); err != nil {
			os.Remove(outPath)
			fatal("failed to copy Windows 3.1 files onto the new container: %v", err)
		}
	}

	eprintln("Restoring Windows 3.1 file attributes...")
	image := mtoolsImageArg(outPath, partStartSector*512)
	applyAttrManifest(image, attrManifest, "")

	var gameName, runPath string
	if archive != "" {
		gameName = dosShortName(archive)
		eprintln()
		eprintf("Injecting %s into \\%s ...\n", archive, gameName)

		gameStage, err := mktempBig()
		if err == nil {
			gameDir := filepath.Join(gameStage, gameName)
			_ = os.MkdirAll(gameDir, 0755)
			if isDir(archive) {
				_ = copyDirTree(archive, gameDir)
			} else {
				_ = extractArchive(archive, gameDir)
			}
			_ = flattenWrapperDirs(gameDir)
			launchExe, launchSubdir, _ := selectLaunchExecutable(gameDir)

			var archAttr map[string]byte
			if classifyArchive(archive) == archZip {
				archAttr, _ = zipAttrManifest(archive)
			}

			if err := loopCopyIn(outPath, partStartSector*512, gameName, []string{gameDir + "/."}, true); err != nil {
				eprintf("Error: failed to copy %s onto the VHD: %v\n", archive, err)
			} else {
				applyAttrManifest(image, archAttr, gameName)
				eprintf("Game files extracted to \\%s\n", gameName)

				if launchExe != "" {
					eprintln("Setting " + launchExe + " to auto-launch when Windows starts...")
					// Single backslashes: this is written straight into
					// WIN.INI's bytes, with no intermediate string-literal
					// parsing step to collapse doubled separators (unlike
					// the original bash, which built this with doubled
					// backslashes specifically so a later python string
					// literal would collapse them back to single ones).
					runPath = "C:\\" + gameName
					if launchSubdir != "" {
						runPath += "\\" + launchSubdir
					}
					runPath += "\\" + launchExe

					winiTmp, terr := os.CreateTemp("", "winini_")
					if terr == nil {
						winiTmp.Close()
						_ = mcopyOut(image, "WINDOWS/WIN.INI", winiTmp.Name())
						content, rerr := os.ReadFile(winiTmp.Name())
						if rerr == nil {
							old := "run=\r\n"
							if strings.Count(string(content), old) == 1 {
								newContent := strings.Replace(string(content), old, "run="+runPath+"\r\n", 1)
								_ = os.WriteFile(winiTmp.Name(), []byte(newContent), 0644)
								_ = mcopyPut(image, []string{winiTmp.Name()}, "WINDOWS/WIN.INI")
							} else {
								eprintln("Warning: could not find an empty run= line in WIN.INI to update.")
							}
						}
						os.Remove(winiTmp.Name())
					}
				}
			}
			os.RemoveAll(gameStage)
		}

		promptDeleteSource(archive)
	}

	// Boot sector merge happens LAST, after every loop-device operation
	// (the main content copy above, and the archive/game injection copy
	// just above it if present) is completely finished -- not right
	// after formatAndFixBPB, where it originally ran. Identical fix (and
	// full explanation) as cmdVHDResize/vhdresize.go: a direct raw-file
	// write done before a later loop-attach/mount cycle on this
	// hardware's kernel/loop-driver combination can be silently
	// clobbered when that later mount gets a stale view of the file.
	// applyAttrManifest calls above use mtools' direct byte-offset
	// access (never a kernel loop device), so they're unaffected either
	// way; only writeBootSectorMerge's direct os.File write needed to
	// move.
	eprintln("Writing boot sector (preserving original boot code)...")
	if err := writeBootSectorMerge(outPath, partStartSector, bootOrig); err != nil {
		os.Remove(outPath)
		fatal("failed to write boot sector: %v", err)
	}

	eprintln()
	eprintln("=======================================================")
	eprintf(" Done! Windows 3.1 VHD ready: %s\n", outPath)
	if archive != "" {
		eprintf(" Game installed to: \\%s\n", gameName)
		if runPath != "" {
			eprintf(" Will auto-launch: %s\n", runPath)
		}
	}
	eprintln("=======================================================")
}
