package main

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// archiveKind classifies a supported archive by filename, matching
// the *.[Zz][Ii][Pp] style case-insensitive globs used throughout the
// original bash scripts.
type archiveKind int

const (
	archUnsupported archiveKind = iota
	archZip
	archTarGz
	archTarBz2
	archTar
)

func classifyArchive(path string) archiveKind {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return archZip
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return archTarGz
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return archTarBz2
	case strings.HasSuffix(lower, ".tar"):
		return archTar
	default:
		return archUnsupported
	}
}

func isSupportedArchive(path string) bool {
	return classifyArchive(path) != archUnsupported
}

// extractArchive extracts a supported archive (.zip, .tar,
// .tar.gz/.tgz, .tar.bz2/.tbz2) into dest. Mirrors extract_archive().
func extractArchive(archivePath, dest string) error {
	switch classifyArchive(archivePath) {
	case archZip:
		return extractZip(archivePath, dest)
	case archTarGz:
		return extractTar(archivePath, dest, "gz")
	case archTarBz2:
		return extractTar(archivePath, dest, "bz2")
	case archTar:
		return extractTar(archivePath, dest, "")
	default:
		return fmt.Errorf("unsupported archive type: %s\nSupported: .zip, .tar, .tar.gz/.tgz, .tar.bz2/.tbz2", archivePath)
	}
}

func extractZip(archivePath, dest string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dest, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(archivePath, dest, comp string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f
	switch comp {
	case "gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	case "bz2":
		r = bzip2.NewReader(f)
	}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// archiveUncompressedSize estimates the uncompressed byte size of a
// supported archive's contents from its own metadata (no extraction).
// Mirrors archive_uncompressed_size().
func archiveUncompressedSize(archivePath string) int64 {
	switch classifyArchive(archivePath) {
	case archZip:
		r, err := zip.OpenReader(archivePath)
		if err != nil {
			return 0
		}
		defer r.Close()
		var total int64
		for _, f := range r.File {
			total += int64(f.UncompressedSize64)
		}
		return total
	case archTarGz:
		return tarUncompressedSize(archivePath, "gz")
	case archTarBz2:
		return tarUncompressedSize(archivePath, "bz2")
	case archTar:
		return tarUncompressedSize(archivePath, "")
	default:
		return 0
	}
}

func tarUncompressedSize(archivePath, comp string) int64 {
	f, err := os.Open(archivePath)
	if err != nil {
		return 0
	}
	defer f.Close()
	var r io.Reader = f
	switch comp {
	case "gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return 0
		}
		defer gz.Close()
		r = gz
	case "bz2":
		r = bzip2.NewReader(f)
	}
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if hdr.Typeflag == tar.TypeReg {
			total += hdr.Size
		}
	}
	return total
}

// flattenWrapperDirs collapses redundant single-subdirectory wrapper
// levels (e.g. Dragonflight/Dragonflight/*), up to 5 levels deep,
// moving the innermost level's contents up to dest. Mirrors
// flatten_wrapper_dirs().
func flattenWrapperDirs(dest string) error {
	cur := dest
	for i := 0; i < 5; i++ {
		entries, err := os.ReadDir(cur)
		if err != nil || len(entries) != 1 {
			break
		}
		fi, err := os.Stat(filepath.Join(cur, entries[0].Name()))
		if err != nil || !fi.IsDir() {
			break
		}
		cur = filepath.Join(cur, entries[0].Name())
	}
	if cur == dest {
		return nil
	}
	// Staged as a SIBLING of dest (same parent directory), not the
	// default temp dir -- dest can live under the big-archive scratch
	// area on /media/fat, and os.Rename (unlike the mv *command*, which
	// silently falls back to copy+delete) fails outright with "invalid
	// cross-device link" when source and destination are on different
	// filesystems/mounts. Staying in the same directory guarantees
	// they're always on the same filesystem.
	staging, err := os.MkdirTemp(filepath.Dir(dest), "flatten_")
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(cur)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(cur, e.Name()), filepath.Join(staging, e.Name())); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(staging, dest)
}

// zipAttrManifest replicates mkvhd/mkvhd(-win31)'s inline python zip
// attribute scanner: for a .zip archive that genuinely originated on
// an MS-DOS/FAT host (creator system 0), captures each file's DOS
// attribute byte, keyed by its path with the same redundant wrapper
// prefix flattenWrapperDirs would strip already removed, so the keys
// line up with post-extraction, post-flatten paths. Archives zipped
// on Unix/macOS/Windows tooling (the vast majority of abandonware
// downloads) carry that OS's own permission bits in the same field
// instead -- reinterpreting those as DOS attributes previously made
// ordinary files show up Hidden+System. Only .zip has this concept;
// .tar has no DOS-attribute metadata to lose.
func zipAttrManifest(archivePath string) (map[string]byte, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	prefix := detectZipWrapperPrefix(names)

	manifest := map[string]byte{}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		system := f.CreatorVersion >> 8 // 0 = MS-DOS/FAT origin
		if system != 0 {
			continue
		}
		attr := byte((f.ExternalAttrs >> 16) & 0xFF)
		if attr&0x07 == 0 {
			continue
		}
		rel := f.Name
		if prefix != "" && strings.HasPrefix(rel, prefix+"/") {
			rel = rel[len(prefix)+1:]
		}
		manifest[rel] = attr
	}
	return manifest, nil
}

// detectZipWrapperPrefix mirrors the python prefix-detection loop:
// repeatedly find the sole top-level path component shared by every
// remaining name, up to 5 levels, stopping as soon as a level isn't
// unanimous or there's nothing left underneath it.
func detectZipWrapperPrefix(names []string) string {
	var prefixParts []string
	cur := names
	for i := 0; i < 5; i++ {
		tops := map[string]bool{}
		for _, n := range cur {
			if n == "" {
				continue
			}
			tops[strings.SplitN(n, "/", 2)[0]] = true
		}
		if len(tops) != 1 {
			break
		}
		var only string
		for t := range tops {
			only = t
		}
		var remainder []string
		for _, n := range cur {
			if strings.HasPrefix(n, only+"/") && len(n) > len(only)+1 {
				remainder = append(remainder, n[len(only)+1:])
			}
		}
		if len(remainder) == 0 {
			break
		}
		prefixParts = append(prefixParts, only)
		cur = remainder
	}
	return strings.Join(prefixParts, "/")
}

// dirSizeBytes sums the apparent size of every regular file under
// root (equivalent to `du -sb`, used for template/staging size
// measurements).
func dirSizeBytes(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
