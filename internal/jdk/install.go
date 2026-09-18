package jdk

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

type Provider interface {
	ArchiveURL(major int, goos, goarch string) (string, error)
}

var providers = map[string]Provider{
	"temurin": temurinProvider{},
}

func ProviderFor(distro string) (Provider, bool) {
	p, ok := providers[distro]
	return p, ok
}

func SupportedDistros() []string {
	out := make([]string, 0, len(providers))
	for name := range providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

var distroArgRe = regexp.MustCompile(`^([a-z]+)(\d+)$`)

func ParseDistroArg(arg string) (distro string, major int, ok bool) {
	m := distroArgRe.FindStringSubmatch(arg)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return m[1], n, true
}

var TemurinAPIBase = "https://api.adoptium.net"

type temurinProvider struct{}

func (temurinProvider) ArchiveURL(major int, goos, goarch string) (string, error) {
	osName, ok := map[string]string{"linux": "linux", "darwin": "mac", "windows": "windows"}[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS %q", goos)
	}
	arch, ok := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
	return fmt.Sprintf("%s/v3/binary/latest/%d/ga/%s/%s/jdk/hotspot/normal/eclipse", TemurinAPIBase, major, osName, arch), nil
}

func DefaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".barista", "toolchains", "jdk"), nil
}

func InstallDir(cfgValue string) (string, error) {
	if cfgValue != "" {
		return cfgValue, nil
	}
	return DefaultInstallDir()
}
