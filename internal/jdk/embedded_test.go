package jdk

import (
	"context"
	"strings"
	"testing"
)

func TestEmbeddedDataLoads(t *testing.T) {
	f, err := loadEmbedded()
	if err != nil {
		t.Fatalf("loadEmbedded: %v", err)
	}
	if len(f.Distros["zulu"]) == 0 {
		t.Fatal("distros.json has no zulu data")
	}
	for _, r := range f.Distros["zulu"] {
		if r.Version == "" || len(r.Assets) == 0 {
			t.Errorf("zulu %d: incomplete release %+v", r.Major, r)
		}
		for platform, a := range r.Assets {
			if !strings.HasPrefix(a.URL, "https://") {
				t.Errorf("zulu %d %s: bad url %q", r.Major, platform, a.URL)
			}
		}
	}
}

func TestEmbeddedArchiveURL(t *testing.T) {
	p := embeddedProvider{"zulu"}
	url, err := p.ArchiveURL(17, "windows", "amd64")
	if err != nil || !strings.Contains(url, "win_x64.zip") {
		t.Errorf("windows/amd64 = %q, %v", url, err)
	}
	if _, err := p.ArchiveURL(17, "plan9", "amd64"); err == nil {
		t.Error("unsupported platform should error")
	}
	if _, err := p.ArchiveURL(9, "linux", "amd64"); err == nil {
		t.Error("unsupported major should error")
	}
	if _, err := (embeddedProvider{"nosuch"}).ArchiveURL(17, "linux", "amd64"); err == nil {
		t.Error("unknown distro should error")
	}
}

func TestEmbeddedAvailable(t *testing.T) {
	releases, e := embeddedProvider{"zulu"}.Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	for i := 1; i < len(releases); i++ {
		if releases[i].Major < releases[i-1].Major {
			t.Fatalf("not sorted by major: %v", releases)
		}
	}
	latest := 0
	for _, r := range releases {
		if r.LTS && r.Major > latest {
			latest = r.Major
		}
	}
	for _, r := range releases {
		if want := r.Major == latest; r.Latest != want {
			t.Errorf("major %d Latest = %v, want %v", r.Major, r.Latest, want)
		}
	}
	if _, e := (embeddedProvider{"nosuch"}).Available(context.Background()); e == nil {
		t.Error("unknown distro should return JDK_AVAILABLE_FAILED")
	}
}
