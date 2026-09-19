package jdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/output"
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

func TestVerifySHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	if e := VerifySHA256(p, hex.EncodeToString(sum[:])); e != nil {
		t.Errorf("match: %v", e)
	}
	e := VerifySHA256(p, "0000")
	if e == nil || e.Code != output.CodeJDKChecksumMismatch {
		t.Errorf("mismatch: got %v", e)
	}
}

func TestGraalVMEmbedded(t *testing.T) {
	p := embeddedProvider{"graalvm"}
	url, err := p.ArchiveURL(25, "windows", "amd64")
	if err != nil || !strings.Contains(url, "graalvm-community-jdk") || !strings.HasSuffix(url, "_bin.zip") {
		t.Errorf("windows/amd64 = %q, %v", url, err)
	}
	sum, ok := p.ExpectedSHA256(25, "windows", "amd64")
	if !ok || len(sum) != 64 {
		t.Errorf("graalvm 25 windows/amd64 sha256 = %q, %v", sum, ok)
	}
	if _, ok := p.ExpectedSHA256(25, "plan9", "amd64"); ok {
		t.Error("unsupported platform should have no checksum")
	}
	releases, e := p.Available(context.Background())
	if e != nil || len(releases) == 0 {
		t.Fatalf("Available: %v", e)
	}
	var r17 AvailableRelease
	for _, r := range releases {
		if r.Major == 17 {
			r17 = r
		}
	}
	if r17.Version == "" || !r17.LTS {
		t.Errorf("graalvm 17 = %+v", r17)
	}
}

func TestZuluNoChecksum(t *testing.T) {
	if _, ok := (embeddedProvider{"zulu"}).ExpectedSHA256(17, "windows", "amd64"); ok {
		t.Error("zulu data carries no sha256")
	}
}
