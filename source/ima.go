package main

import (
	"os"
	"path/filepath"
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

// cmdIMACreate implements `aotools ima create <name.ima> [source]
// [-s size]`, porting mkima in full.
func cmdIMACreate(args []string) {
	if len(args) == 0 {
		eprintln("Usage:")
		eprintln("  aotools ima create <name.ima>                      blank formatted floppy (1.44MB default)")
		eprintln("  aotools ima create <name.ima> -s <size>            blank formatted floppy of a given size")
		eprintln("  aotools ima create <name.ima> <source>             inject a directory or archive")
		eprintln("  aotools ima create <name.ima> <source> -s <size>   same, with an explicit size")
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

	var sizeBytes int64
	if sizeSpec != "" {
		b, ok := imaSizeToBytes(sizeSpec)
		if !ok {
			eprintf("Error: unsupported size '%s'.\n", sizeSpec)
			eprintln("Supported: 360k, 720k, 1.2m, 1.44m, 2.88m")
			os.Exit(1)
		}
		sizeBytes = b
	} else if source != "" {
		var sourceBytes int64
		if isDir(source) {
			sourceBytes = dirSizeBytes(source)
		} else {
			sourceBytes = archiveUncompressedSize(source)
		}
		found := false
		for _, candidate := range []int64{368640, 737280, 1228800, 1474560, 2949120} {
			// ~5% headroom for filesystem overhead.
			if sourceBytes <= candidate*95/100 {
				sizeBytes = candidate
				found = true
				break
			}
		}
		if !found {
			eprintf("Error: source (%d bytes) is too large for any\n", sourceBytes)
			eprintln("standard floppy size (max 2.88MB / 2,949,120 bytes).")
			os.Exit(1)
		}
	} else {
		sizeBytes = 1474560
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
