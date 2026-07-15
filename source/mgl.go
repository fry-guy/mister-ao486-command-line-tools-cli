package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var mglHDDExts = map[string]bool{".vhd": true}
var mglCDExts = map[string]bool{".chd": true, ".iso": true, ".cue": true}
var mglFloppyExts = map[string]bool{".img": true, ".ima": true, ".dsk": true, ".vfd": true, ".imz": true, ".flp": true}

// cmdMGLCreate implements `aotools mgl create -dos|-win31 "Display
// Name" [source_folder]`, porting mkmgl in full.
func cmdMGLCreate(args []string) {
	if len(args) < 2 {
		mglUsage()
	}
	typeFlag := args[0]
	var targetDir string
	switch typeFlag {
	case "-dos":
		targetDir = dosGamesDir
	case "-win31":
		targetDir = win31GamesDir
	default:
		fmt.Fprintf(os.Stderr, "Error: unrecognized type flag '%s'\n", typeFlag)
		mglUsage()
	}

	displayName := args[1]
	sourceFolder := "."
	if len(args) > 2 {
		sourceFolder = args[2]
	}
	abs, err := filepath.Abs(sourceFolder)
	if err != nil {
		fatal("%v", err)
	}
	sourceFolder = abs
	if !isDir(sourceFolder) {
		fatal("source folder not found: %s", sourceFolder)
	}

	hdd, cd, floppy := mglScanCandidates(sourceFolder)
	if len(hdd) == 0 && len(cd) == 0 && len(floppy) == 0 {
		eprintf("Error: no recognized disk image files found in %s\n", sourceFolder)
		eprintln("(looked for: .chd, .cue, .dsk, .flp, .ima, .img, .imz, .iso, .vfd, .vhd)")
		os.Exit(1)
	}

	hddChoice := mglChoose(hdd, "hard drive (.vhd)")
	cdChoice := mglChoose(cd, "CD image")
	floppyChoice := mglChoose(floppy, "floppy image")

	var lines []string
	lines = append(lines, "<mistergamedescription>", "    <rbf>_computer/ao486</rbf>")
	if floppyChoice != "" {
		lines = append(lines, fmt.Sprintf(`    <file delay="0" type="s" index="0" path="%s"/>`, mglRelPath(sourceFolder, floppyChoice)))
	}
	if hddChoice != "" {
		lines = append(lines, fmt.Sprintf(`    <file delay="0" type="s" index="2" path="%s"/>`, mglRelPath(sourceFolder, hddChoice)))
	}
	if cdChoice != "" {
		lines = append(lines, fmt.Sprintf(`    <file delay="0" type="s" index="4" path="%s"/>`, mglRelPath(sourceFolder, cdChoice)))
	}
	lines = append(lines, `    <reset delay="1"/>`, "</mistergamedescription>")
	content := strings.Join(lines, "\n") + "\n"

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fatal("%v", err)
	}
	outPath := filepath.Join(targetDir, displayName+".mgl")

	if fileExists(outPath) {
		answer := strings.ToLower(promptLine(fmt.Sprintf("%s already exists. Overwrite? [y/N]: ", outPath)))
		if answer != "y" {
			eprintln("Aborted; nothing was written.")
			return
		}
	}

	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		fatal("writing %s: %v", outPath, err)
	}

	eprintln()
	eprintf("Wrote: %s\n", outPath)
	if hddChoice != "" {
		eprintf("  Hard drive: %s\n", hddChoice)
	}
	if cdChoice != "" {
		eprintf("  CD image:   %s\n", cdChoice)
	}
	if floppyChoice != "" {
		eprintf("  Floppy:     %s\n", floppyChoice)
	}
	eprintln()
	fmt.Print(content)
}

func mglUsage() {
	eprintln(`Usage: aotools mgl create -dos|-win31 "Display Name" [source_folder]`)
	eprintln()
	eprintln("  -dos          Place the .mgl in /media/fat/_DOS Games")
	eprintln("  -win31        Place the .mgl in /media/fat/_Win 3.1 Games")
	eprintln()
	eprintln("  source_folder defaults to the current directory if omitted.")
	os.Exit(1)
}

func mglScanCandidates(folder string) (hdd, cd, floppy []string) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		fatal("cannot read folder %s: %v", folder, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		full := filepath.Join(folder, name)
		fi, err := os.Stat(full)
		if err != nil || fi.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch {
		case mglHDDExts[ext]:
			hdd = append(hdd, name)
		case mglCDExts[ext]:
			cd = append(cd, name)
		case mglFloppyExts[ext]:
			floppy = append(floppy, name)
		}
	}

	// A .chd always wins over .iso/.cue if one exists.
	hasCHD := false
	for _, n := range cd {
		if strings.HasSuffix(strings.ToLower(n), ".chd") {
			hasCHD = true
			break
		}
	}
	if hasCHD {
		var onlyCHD []string
		for _, n := range cd {
			if strings.HasSuffix(strings.ToLower(n), ".chd") {
				onlyCHD = append(onlyCHD, n)
			}
		}
		cd = onlyCHD
	}
	return
}

func mglChoose(candidates []string, label string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	eprintln()
	eprintf("Multiple %s files found:\n", label)
	for i, name := range candidates {
		eprintf("  %d. %s\n", i+1, name)
	}
	for {
		choiceStr := promptLine(fmt.Sprintf("Select the %s to use [1-%d]: ", label, len(candidates)))
		choice, err := strconv.Atoi(choiceStr)
		if err == nil && choice >= 1 && choice <= len(candidates) {
			return candidates[choice-1]
		}
		eprintln("Invalid choice, try again.")
	}
}

// mglRelPath returns filename's path relative to the ao486 media
// root, which is what the core resolves file="" paths against. Falls
// back to an absolute path (with a warning) if sourceFolder isn't
// actually under it.
func mglRelPath(sourceFolder, filename string) string {
	full := filepath.Join(sourceFolder, filename)
	rel, err := filepath.Rel(gamesRoot, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		eprintf("Warning: %s\n", full)
		eprintf("is not inside %s; using an absolute path instead of a relative one.\n", gamesRoot)
		return full
	}
	return filepath.ToSlash(rel)
}
