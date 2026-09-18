package jdk

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDistroArg(t *testing.T) {
	cases := []struct {
		in     string
		distro string
		major  int
		ok     bool
	}{
		{"temurin17", "temurin", 17, true},
		{"openjdk9", "openjdk", 9, true},
		{"temurin", "", 0, false},
		{"17", "", 0, false},
		{"Temurin17", "", 0, false},
		{"temurin17-lts", "", 0, false},
		{"temurin0", "", 0, false},
	}
	for _, tc := range cases {
		d, m, ok := ParseDistroArg(tc.in)
		if d != tc.distro || m != tc.major || ok != tc.ok {
			t.Errorf("ParseDistroArg(%q) = %q, %d, %v; want %q, %d, %v", tc.in, d, m, ok, tc.distro, tc.major, tc.ok)
		}
	}
}

func TestTemurinArchiveURL(t *testing.T) {
	p := temurinProvider{}
	url, err := p.ArchiveURL(17, "linux", "amd64")
	if err != nil || !strings.Contains(url, "/ga/linux/x64/") {
		t.Errorf("linux/amd64 = %q, %v", url, err)
	}
	url, err = p.ArchiveURL(21, "darwin", "arm64")
	if err != nil || !strings.Contains(url, "/ga/mac/aarch64/") {
		t.Errorf("darwin/arm64 = %q, %v", url, err)
	}
	if _, err := p.ArchiveURL(17, "plan9", "amd64"); err == nil {
		t.Error("unsupported OS should error")
	}
	if _, err := p.ArchiveURL(17, "linux", "386"); err == nil {
		t.Error("unsupported arch should error")
	}
}

func TestInstallDir(t *testing.T) {
	if got, err := InstallDir(`D:\jdks`); err != nil || got != `D:\jdks` {
		t.Errorf("InstallDir(cfg) = %q, %v", got, err)
	}
	got, err := InstallDir("")
	if err != nil {
		t.Fatalf("InstallDir(default): %v", err)
	}
	if !strings.Contains(filepath.ToSlash(got), ".barista/toolchains/jdk") {
		t.Errorf("default install dir = %q", got)
	}
}
