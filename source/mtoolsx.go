package main

import (
	"fmt"
	"os/exec"
)

func mtoolsImageArg(path string, offsetBytes int64) string {
	if offsetBytes == 0 {
		return path
	}
	return fmt.Sprintf("%s@@%d", path, offsetBytes)
}

func mcopyPut(image string, sources []string, destOnImage string) error {
	args := []string{"-D", "o", "-D", "O", "-i", image}
	args = append(args, sources...)
	args = append(args, "::"+destOnImage)
	cmd := exec.Command(mcopyBin, args...)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy: %v: %s", err, out)
	}
	return nil
}

func mcopyPutRecursive(image string, sources []string, destOnImage string) error {
	args := []string{"-s", "-D", "o", "-D", "O", "-i", image}
	args = append(args, sources...)
	args = append(args, "::"+destOnImage)
	cmd := exec.Command(mcopyBin, args...)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy -s: %v: %s", err, out)
	}
	return nil
}

func mcopyOut(image, srcOnImage, localDest string) error {
	cmd := exec.Command(mcopyBin, "-n", "-i", image, "::"+srcOnImage, localDest)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy: %v: %s", err, out)
	}
	return nil
}

func mcopyOutTree(image, localDest string) error {
	cmd := exec.Command(mcopyBin, "-s", "-i", image, "::", localDest)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mcopy -s: %v: %s", err, out)
	}
	return nil
}

func mattribSet(image, pathOnImage string, flags []string) error {
	if len(flags) == 0 {
		return nil
	}
	args := append(append([]string{}, flags...), "-i", image, "::"+pathOnImage)
	cmd := exec.Command(mattribBin, args...)
	cmd.Stdin = nil
	_, _ = cmd.CombinedOutput()
	return nil
}

func applyAttrManifest(image string, manifest map[string]byte, prefix string) {
	for fname, attr := range manifest {
		flags := mattribFlagsFor(attr)
		if len(flags) == 0 {
			continue
		}
		p := fname
		if prefix != "" {
			p = prefix + "/" + fname
		}
		_ = mattribSet(image, p, flags)
	}
}
