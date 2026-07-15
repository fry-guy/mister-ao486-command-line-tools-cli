package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]`)

// dosShortName derives a DOS 6.22-compatible 8.3 short name (no
// extension) from an archive path: strip directory + known archive
// extensions, uppercase, strip invalid chars, truncate to "6+~1" if
// over 8 characters. Mirrors dos_short_name() in mkvhd/mkima.
func dosShortName(raw string) string {
	base := filepath.Base(raw)
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		base = base[:len(base)-len(".tar.gz")]
	case strings.HasSuffix(lower, ".tgz"):
		base = base[:len(base)-len(".tgz")]
	case strings.HasSuffix(lower, ".tar.bz2"):
		base = base[:len(base)-len(".tar.bz2")]
	case strings.HasSuffix(lower, ".tbz2"):
		base = base[:len(base)-len(".tbz2")]
	case strings.HasSuffix(lower, ".zip"):
		base = base[:len(base)-len(".zip")]
	case strings.HasSuffix(lower, ".tar"):
		base = base[:len(base)-len(".tar")]
	default:
		if ext := filepath.Ext(base); ext != "" {
			base = base[:len(base)-len(ext)]
		}
	}
	base = strings.ToUpper(base)
	base = nonAlnum.ReplaceAllString(base, "")
	if base == "" {
		base = "GAME"
	}
	if len(base) > 8 {
		return base[:6] + "~1"
	}
	return base
}

// dosShortFilename is like dosShortName but for an individual file:
// preserves the extension (uppercased, max 3 chars) separately.
// Mirrors dos_short_filename().
func dosShortFilename(raw string) string {
	name := filepath.Base(raw)
	var base, ext string
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		base, ext = name[:idx], name[idx+1:]
	} else {
		base, ext = name, ""
	}
	base = nonAlnum.ReplaceAllString(strings.ToUpper(base), "")
	ext = nonAlnum.ReplaceAllString(strings.ToUpper(ext), "")
	if len(ext) > 3 {
		ext = ext[:3]
	}
	if base == "" {
		base = "FILE"
	}
	if len(base) > 8 {
		base = base[:6] + "~1"
	}
	if ext != "" {
		return base + "." + ext
	}
	return base
}

// dosShortDirname is like dosShortName but for a single directory
// path component (no extension handling). Mirrors dos_short_dirname().
func dosShortDirname(raw string) string {
	base := nonAlnum.ReplaceAllString(strings.ToUpper(raw), "")
	if base == "" {
		base = "DIR"
	}
	if len(base) > 8 {
		return base[:6] + "~1"
	}
	return base
}

// resolveShortComponent resolves a single path component (file or
// directory) under dir to a safe DOS 8.3 short name, renaming it on
// disk if needed. Falls back to the original name only on a genuine
// collision with a DIFFERENT existing entry. Mirrors
// resolve_short_component().
func resolveShortComponent(original, dir string, isDirEntry bool) (string, error) {
	var shortname string
	if isDirEntry {
		shortname = dosShortDirname(original)
	} else {
		shortname = dosShortFilename(original)
	}
	if original == shortname {
		return shortname, nil
	}

	origPath := filepath.Join(dir, original)
	shortPath := filepath.Join(dir, shortname)

	if fileExists(shortPath) {
		origInfo, err1 := os.Stat(origPath)
		shortInfo, err2 := os.Stat(shortPath)
		if err1 == nil && err2 == nil && os.SameFile(origInfo, shortInfo) {
			return shortname, nil
		}
		eprintf("Warning: %s already exists under %s; using %s's original name instead.\n",
			shortname, filepath.Base(dir), original)
		return original, nil
	}

	if err := os.Rename(origPath, shortPath); err != nil {
		return "", err
	}
	eprintf("Renamed %s -> %s (DOS 8.3 short name)\n", original, shortname)
	return shortname, nil
}

// selectLaunchExecutable scans gamedir (recursively) for candidate
// launch executables (.exe/.bat/.com), shows a numbered picker on
// stderr, prompts on stdin, and resolves the chosen path's components
// to DOS 8.3 short names (renaming on disk as needed). Mirrors
// select_launch_executable(). Returns (launchExe, launchSubdir).
// launchSubdir is backslash-joined; both are empty if nothing was
// chosen or no candidates exist.
func selectLaunchExecutable(gamedir string) (launchExe, launchSubdir string, err error) {
	var candidates []string
	err = filepath.Walk(gamedir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		lower := strings.ToLower(info.Name())
		if strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".com") {
			rel, relErr := filepath.Rel(gamedir, path)
			if relErr == nil {
				candidates = append(candidates, rel)
			}
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if len(candidates) == 0 {
		return "", "", nil
	}
	sort.Strings(candidates)

	eprintln()
	eprintln("Select the executable to launch on startup:")
	for i, f := range candidates {
		eprintf("  %d. %s\n", i+1, strings.ReplaceAll(f, "/", "\\"))
	}
	noneChoice := len(candidates) + 1
	eprintf("  %d. None of the above\n", noneChoice)

	choiceStr := promptLine("Enter choice [" + strconv.Itoa(noneChoice) + "]: ")
	if choiceStr == "" {
		choiceStr = strconv.Itoa(noneChoice)
	}
	choice, convErr := strconv.Atoi(choiceStr)
	if convErr != nil || choice < 1 || choice > noneChoice {
		eprintln("Invalid choice; not adding a launch executable.")
		return "", "", nil
	}
	if choice == noneChoice {
		return "", "", nil
	}

	chosenRel := candidates[choice-1]
	dirPart := filepath.Dir(chosenRel)
	filePart := filepath.Base(chosenRel)

	cur := gamedir
	var shortComponents []string
	if dirPart != "." {
		for _, part := range strings.Split(dirPart, "/") {
			shortPart, rerr := resolveShortComponent(part, cur, true)
			if rerr != nil {
				return "", "", rerr
			}
			shortComponents = append(shortComponents, shortPart)
			cur = filepath.Join(cur, shortPart)
		}
	}

	shortFile, rerr := resolveShortComponent(filePart, cur, false)
	if rerr != nil {
		return "", "", rerr
	}
	launchExe = shortFile
	if len(shortComponents) > 0 {
		launchSubdir = strings.Join(shortComponents, "\\")
	}
	return launchExe, launchSubdir, nil
}
