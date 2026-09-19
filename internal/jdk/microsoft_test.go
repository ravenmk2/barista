package jdk

import (
	"context"
	"strings"
	"testing"
)

func TestMicrosoftArchiveURL(t *testing.T) {
	p := microsoftProvider{}
	url, err := p.ArchiveURL(17, "windows", "amd64")
	if err != nil || url != "https://aka.ms/download-jdk/microsoft-jdk-17-windows-x64.zip" {
		t.Errorf("windows/amd64 = %q, %v", url, err)
	}
	url, err = p.ArchiveURL(21, "darwin", "arm64")
	if err != nil || url != "https://aka.ms/download-jdk/microsoft-jdk-21-macos-aarch64.tar.gz" {
		t.Errorf("darwin/arm64 = %q, %v", url, err)
	}
	if _, err := p.ArchiveURL(11, "linux", "amd64"); err == nil {
		t.Error("retired major 11 should error")
	}
	if _, err := p.ArchiveURL(17, "plan9", "amd64"); err == nil {
		t.Error("unsupported OS should error")
	}
	if _, err := p.ArchiveURL(17, "linux", "386"); err == nil {
		t.Error("unsupported arch should error")
	}
}

func TestMicrosoftAvailable(t *testing.T) {
	releases, e := microsoftProvider{}.Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	if len(releases) != len(microsoftMajors) {
		t.Fatalf("got %d releases, want %d", len(releases), len(microsoftMajors))
	}
	for i, r := range releases {
		if r.Major != microsoftMajors[i] || !r.LTS {
			t.Errorf("releases[%d] = %+v", i, r)
		}
		if r.Version != "" {
			t.Errorf("permalink provider should leave Version empty, got %q", r.Version)
		}
	}
	if !releases[len(releases)-1].Latest {
		t.Error("last major should be marked latest")
	}
}

func TestStaticAvailableOrder(t *testing.T) {
	releases := staticAvailable([]int{8, 17}, 17)
	if releases[0].Major != 8 || releases[1].Major != 17 {
		t.Errorf("order = %v", releases)
	}
	if !releases[0].LTS || releases[0].Latest || !releases[1].Latest {
		t.Errorf("flags = %+v", releases)
	}
}

func TestMicrosoftRegistered(t *testing.T) {
	if _, ok := ProviderFor("microsoft"); !ok {
		t.Error("microsoft provider not registered")
	}
	if !strings.Contains(strings.Join(SupportedDistros(), ","), "microsoft") {
		t.Errorf("SupportedDistros = %v", SupportedDistros())
	}
}
