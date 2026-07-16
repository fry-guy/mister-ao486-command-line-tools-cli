package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func imaSizeToBytes(spec string) (int64, bool) {
	switch strings.ToLower(spec) {
	case "360k":
		return 368640, true
	case "720k":
		return 737280, true
	case "1.2m":
		return 1228800, true
	case "1.44m":
		return 1474560, true
	case "2.88m":
		return 2949120, true
	default:
		return 0, false
	}
}

type diskimageSize struct {
	label string
	bytes int64
}

var diskimageSizes = []diskimageSize{
	{"360k", 368640},
	{"720k", 737280},
	{"1.2m", 1228800},
	{"1.44m", 1474560},
	{"2.88m", 2949120},
}

const diskimageDefaultIdx = 3 // 1.44m

// promptDiskimageSize shows a numbered list of the standard floppy
// sizes and returns the chosen size in bytes. sourceBytes is the size
// of the content to be injected (0 if none), used to flag a
// recommended size that fits it. Blank input selects the default,
// 1.44MB. A raw size string (e.g. "2.88m") is also accepted directly.
func promptDiskimageSize(source string, sourceBytes int64) int64 {
	recommended := -1
	if source != "" {
		for i, s := range diskimageSizes {
			if sourceBytes <= s.bytes*95/100 {
				recommended = i
				break
			}
		}
	}

	eprintln()
	eprintln("Select a floppy image size:")
	for i, s := range diskimageSizes {
		suffix := ""
		if i == diskimageDefaultIdx {
			suffix += " (default)"
		}
		if i == recommended {
			suffix += " (recommended for source)"
		}
		eprintf("  %d. %s%s\n", i+1, s.label, suffix)
	}
	answer := promptLine(fmt.Sprintf("Choice [%d]: ", diskimageDefaultIdx+1))
	if answer == "" {
		return diskimageSizes[diskimageDefaultIdx].bytes
	}
	if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(diskimageSizes) {
		return diskimageSizes[n-1].bytes
	}
	if b, ok := imaSizeToBytes(answer); ok {
		return b
	}
	fatal("unrecognized size '%s'. Enter a number 1-%d or a size like 1.44m.", answer, len(diskimageSizes))
	return 0
}

// cmdIMACreate implements `aotools create diskimage <name.ima>
// [source] [-s size]`, porting mkima in full.
func cmdIMACreate(args []string) {
	if len(args) == 0 {
		eprintln("Usage:")
		eprintln("  aotools create diskimage <name.ima>                      blank formatted floppy (prompts for size)")
		eprintln("  aotools create diskimage <name.ima> -s <size>            blank formatted floppy of a given size")
		eprintln("  aotools create diskimage <name.ima> <source>             inject a directory or archive")
		eprintln("  aotools create diskimage <name.ima> <source> -s <size>   same, with an explicit size")
		eprintln()
		eprintln("Sizes: 360k, 720k, 1.2m, 1.44m (default), 2.88m")
		os.Exit(1)
	}

	if err := ensureMtoolsSymlinks(); err != nil {
		fatal("%v", err)
	}

	var outName, source, sizeSpec string
	i := 0
	for i < len(args) {
		if args[i] == "-s" && i+1 < len(args) {
			sizeSpec = args[i+1]
			i += 2
			continue
		}
		if outName == "" {
			outName = args[i]
		} else if source == "" {
			source = args[i]
		} else {
			fatal("unexpected argument: %s", args[i])
		}
		i++
	}

	lower := strings.ToLower(outName)
	if !strings.HasSuffix(lower, ".ima") && !strings.HasSuffix(lower, ".img") {
		outName += ".ima"
	}
	destDir, _ := os.Getwd()
	outPath := filepath.Join(destDir, outName)

	if fileExists(outPath) {
		fatal("%s already exists.", outPath)
	}
	if source != "" && !fileExists(source) {
		fatal("source not found: %s", source)
	}

	// source can be either a folder or an archive (.zip/.tar/.tar.gz/
	// .tar.bz2, per archext.go) -- both are already fully supported
	// by isDir/extractArchive below.
	var sourceBytes int64
	if source != "" {
		if isDir(source) {
			sourceBytes = dirSizeBytes(source)
		} else {
			sourceBytes = archiveUncompressedSize(source)
		}
	}

	var sizeBytes int64
	if sizeSpec != "" {
		b, ok := imaSizeToBytes(sizeSpec)
		if !ok {
			eprintf("Error: unsupported size '%s'.\n", sizeSpec)
			eprintln("Supported: 360k, 720k, 1.2m, 1.44m, 2.88m")
			os.Exit(1)
		}
		sizeBytes = b
	} else {
		sizeBytes = promptDiskimageSize(source, sourceBytes)
	}

	if source != "" && sourceBytes > sizeBytes*95/100 {
		fatal("source (%d bytes) is too large for a %dKB floppy image.", sourceBytes, sizeBytes/1024)
	}

	eprintf("Creating %dKB floppy image...\n", sizeBytes/1024)
	if err := createSparseFile(outPath, sizeBytes); err != nil {
		fatal("%v", err)
	}
	if out, err := runCapture("mkfs.vfat", "-I", outPath); err != nil {
		os.Remove(outPath)
		fatal("mkfs.vfat failed: %s", out)
	}

	if source != "" {
		stageDir := source
		stageIsTemp := false
		if !isDir(source) {
			td, err := os.MkdirTemp("", "mkima_stage_")
			if err != nil {
				os.Remove(outPath)
				fatal("%v", err)
			}
			stageDir = td
			stageIsTemp = true
			if err := extractArchive(source, stageDir); err != nil {
				os.RemoveAll(stageDir)
				os.Remove(outPath)
				fatal("failed to copy %s onto the floppy image (likely too large): %v", source, err)
			}
			_ = flattenWrapperDirs(stageDir)
		}

		entries, _ := os.ReadDir(stageDir)
		if len(entries) > 0 {
			var items []string
			for _, e := range entries {
				items = append(items, filepath.Join(stageDir, e.Name()))
			}
			if err := mcopyPutRecursive(outPath, items, ""); err != nil {
				if stageIsTemp {
					os.RemoveAll(stageDir)
				}
				os.Remove(outPath)
				fatal("failed to copy %s onto the floppy image (likely too large): %v", source, err)
			}
		}
		if stageIsTemp {
			os.RemoveAll(stageDir)
		}
	}

	eprintf("Done: %s\n", outPath)
}
