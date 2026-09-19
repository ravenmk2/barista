package jdk

import (
	"context"
	"fmt"

	"barista/internal/output"
)

type correttoProvider struct{}

// correttoMajors are the Corretto majors with live permalinks. All are LTS.
var correttoMajors = []int{8, 11, 17, 21, 25}

func (correttoProvider) ArchiveURL(major int, goos, goarch string) (string, error) {
	osName, ok := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS %q", goos)
	}
	arch, ok := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
	if goos == "windows" && goarch == "arm64" {
		return "", fmt.Errorf("corretto does not publish windows/aarch64 builds")
	}
	if !hasMajor(correttoMajors, major) {
		return "", fmt.Errorf("unsupported major %d (supported: %v)", major, correttoMajors)
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("https://corretto.aws/downloads/latest/amazon-corretto-%d-%s-%s-jdk.%s", major, arch, osName, ext), nil
}

func (correttoProvider) Available(context.Context) ([]AvailableRelease, *output.ErrInfo) {
	return staticAvailable(correttoMajors, correttoMajors[len(correttoMajors)-1]), nil
}
