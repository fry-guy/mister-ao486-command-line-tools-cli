package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// cmdCHDCreate implements `aotools chd create <input> <output.chd>`,
// porting mkchd in full.
func cmdCHDCreate(args []string) {
	if len(args) < 2 {
		eprintln("Usage: aotools chd create <input.iso|.cue|.bin|.gdi> <output.chd>")
		os.Exit(1)
	}
	input, output := args[0], args[1]
	if !fileExists(input) {
		fatal("input file not found: %s", input)
	}
	if !fileExists(chdmanBin) {
		fatal("chdman not found or not executable at %s", chdmanBin)
	}

	cmd := exec.Command(chdmanBin, "createcd", "-i", input, "-o", output, "-f")
	cmd.Stdin = nil
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	if err != nil {
		exitCode := -1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		if exitCode == 135 {
			eprintln("Error: chdman crashed (Bus error / SIGBUS) while creating " + output + ".")
			eprintln("This shouldn't happen; please don't just retry blindly --")
			eprintln("worth checking the input file and available disk space.")
		} else {
			eprintf("Error: chdman failed (exit code %d) while creating %s.\n", exitCode, output)
		}
		if fileExists(output) {
			os.Remove(output)
			eprintf("Removed incomplete/corrupt output file: %s\n", output)
		}
		os.Exit(1)
	}

	eprintf("Done: %s\n", output)

	// Offer to clean up the original source file(s). For .cue/.gdi/
	// .toc, these are text descriptors that can reference one or more
	// separate data files (e.g. a .cue pointing at a .bin) -- scan the
	// descriptor's own content for every file it references so we
	// don't leave orphaned multi-hundred-MB .bin files behind.
	srcDir := filepath.Dir(input)
	srcBase := filepath.Base(input)
	outAbs, _ := filepath.Abs(output)

	toDelete := []string{input}
	lower := strings.ToLower(srcBase)
	if strings.HasSuffix(lower, ".cue") || strings.HasSuffix(lower, ".gdi") || strings.HasSuffix(lower, ".toc") {
		refs := extractCueReferences(input)
		for _, ref := range refs {
			refPath := filepath.Join(srcDir, ref)
			if !fileExists(refPath) {
				continue
			}
			refAbs, _ := filepath.Abs(refPath)
			if refAbs == outAbs {
				continue
			}
			already := false
			for _, existing := range toDelete {
				existAbs, _ := filepath.Abs(existing)
				if existAbs == refAbs {
					already = true
					break
				}
			}
			if !already {
				toDelete = append(toDelete, refPath)
			}
		}
	}

	eprintln()
	eprintln("Source file(s) that can now be removed:")
	for _, f := range toDelete {
		eprintf("  %s\n", f)
	}
	answer := promptLine("Delete these original source file(s)? [y/N]: ")
	if answer == "y" || answer == "Y" {
		for _, f := range toDelete {
			os.Remove(f)
		}
		eprintln("Deleted.")
	} else {
		eprintln("Keeping source file(s).")
	}
}

var cueQuotedRE = regexp.MustCompile(`"([^"]+)"`)
var cueBinRefRE = regexp.MustCompile(`[A-Za-z0-9_.-]+\.(?i:bin|raw|iso)`)

// extractCueReferences pulls every quoted string and bin/raw/iso
// filename reference out of a .cue/.gdi/.toc file, mirroring the
// grep -oE pipeline in mkchd.
func extractCueReferences(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var refs []string
	for _, m := range cueQuotedRE.FindAllStringSubmatch(string(data), -1) {
		refs = append(refs, m[1])
	}
	for _, m := range cueBinRefRE.FindAllString(string(data), -1) {
		refs = append(refs, m)
	}
	return refs
}
