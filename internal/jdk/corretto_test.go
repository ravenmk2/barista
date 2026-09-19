package jdk

import (
	"context"
	"testing"
)

func TestCorrettoArchiveURL(t *testing.T) {
	p := correttoProvider{}
	url, err := p.ArchiveURL(17, "windows", "amd64")
	if err != nil || url != "https://corretto.aws/downloads/latest/amazon-corretto-17-x64-windows-jdk.zip" {
		t.Errorf("windows/amd64 = %q, %v", url, err)
	}
	url, err = p.ArchiveURL(8, "darwin", "arm64")
	if err != nil || url != "https://corretto.aws/downloads/latest/amazon-corretto-8-aarch64-macos-jdk.tar.gz" {
		t.Errorf("darwin/arm64 = %q, %v", url, err)
	}
	if _, err := p.ArchiveURL(17, "windows", "arm64"); err == nil {
		t.Error("windows/aarch64 is not published, should error")
	}
	if _, err := p.ArchiveURL(9, "linux", "amd64"); err == nil {
		t.Error("unsupported major should error")
	}
	if _, err := p.ArchiveURL(17, "plan9", "amd64"); err == nil {
		t.Error("unsupported OS should error")
	}
}

func TestCorrettoAvailable(t *testing.T) {
	releases, e := correttoProvider{}.Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	if len(releases) != len(correttoMajors) {
		t.Fatalf("got %d releases, want %d", len(releases), len(correttoMajors))
	}
	for i, r := range releases {
		if r.Major != correttoMajors[i] || !r.LTS || r.Version != "" {
			t.Errorf("releases[%d] = %+v", i, r)
		}
	}
	if !releases[len(releases)-1].Latest {
		t.Error("last major should be marked latest")
	}
}

func TestCorrettoRegistered(t *testing.T) {
	if _, ok := ProviderFor("corretto"); !ok {
		t.Error("corretto provider not registered")
	}
}
