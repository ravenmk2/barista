package jdk

import (
	"context"
	"fmt"

	"barista/internal/output"
)

// staticAvailable builds the release list for permalink-based providers whose
// available majors are a fixed set; no network call is involved.
func staticAvailable(majors []int, latest int) []AvailableRelease {
	releases := make([]AvailableRelease, len(majors))
	for i, m := range majors {
		releases[i] = AvailableRelease{Major: m, LTS: true, Latest: m == latest}
	}
	return releases
}

type microsoftProvider struct{}

// microsoftMajors are the majors with live aka.ms permalinks (JDK 11 permalinks
// were retired). All are LTS.
var microsoftMajors = []int{17, 21, 25}

func (microsoftProvider) ArchiveURL(major int, goos, goarch string) (string, error) {
	osName, ok := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS %q", goos)
	}
	arch, ok := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
	if !hasMajor(microsoftMajors, major) {
		return "", fmt.Errorf("unsupported major %d (supported: %v)", major, microsoftMajors)
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("https://aka.ms/download-jdk/microsoft-jdk-%d-%s-%s.%s", major, osName, arch, ext), nil
}

func (microsoftProvider) Available(context.Context) ([]AvailableRelease, *output.ErrInfo) {
	return staticAvailable(microsoftMajors, microsoftMajors[len(microsoftMajors)-1]), nil
}

func hasMajor(majors []int, major int) bool {
	for _, m := range majors {
		if m == major {
			return true
		}
	}
	return false
}
