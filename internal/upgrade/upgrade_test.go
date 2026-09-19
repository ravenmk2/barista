package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"barista/internal/output"
)

const validManifest = `{
  "schemaVersion": 1,
  "version": "v0.3.0",
  "assets": {
    "windows/amd64": {"file": "barista-windows-amd64.exe", "sha256": "abc", "size": 10},
    "linux/amd64": {"file": "barista-linux-amd64", "sha256": "def", "size": 10}
  }
}`

func TestParseManifest(t *testing.T) {
	m, e := ParseManifest([]byte(validManifest))
	if e != nil {
		t.Fatal(e)
	}
	if m.Version != "v0.3.0" {
		t.Errorf("got version %q", m.Version)
	}
	a, ok := m.Asset("windows", "amd64")
	if !ok || a.File != "barista-windows-amd64.exe" {
		t.Errorf("asset lookup failed: %+v", a)
	}
	if _, ok := m.Asset("darwin", "arm64"); ok {
		t.Error("missing asset should not be found")
	}
}

func TestParseManifestInvalid(t *testing.T) {
	for _, data := range []string{
		"not json",
		`{"schemaVersion": 2, "version": "v1", "assets": {"a/b": {}}}`,
		`{"schemaVersion": 1, "assets": {"a/b": {}}}`,
		`{"schemaVersion": 1, "version": "v1"}`,
	} {
		if _, e := ParseManifest([]byte(data)); e == nil || e.Code != output.CodeUpgradeCheckFailed {
			t.Errorf("%s: want UPGRADE_CHECK_FAILED, got %v", data, e)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		newer, known    bool
	}{
		{"v0.2.0", "v0.3.0", true, true},
		{"0.2.0", "v0.3.0", true, true},
		{"v0.3.0", "v0.3.0", false, true},
		{"v0.4.0", "v0.3.0", false, true},
		{"dev", "v0.3.0", true, false},
		{"v0.2.0-5-gabc", "v0.3.0", true, false},
		{"", "v0.3.0", true, false},
	}
	for _, c := range cases {
		newer, known := Newer(c.current, c.latest)
		if newer != c.newer || known != c.known {
			t.Errorf("Newer(%q, %q) = (%v, %v), want (%v, %v)", c.current, c.latest, newer, known, c.newer, c.known)
		}
	}
}

func TestVerifySHA256(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(f, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	if e := VerifySHA256(f, hex.EncodeToString(sum[:])); e != nil {
		t.Errorf("matching sum: %v", e)
	}
	e := VerifySHA256(f, "0000")
	if e == nil || e.Code != output.CodeUpgradeChecksumMismatch {
		t.Errorf("mismatch: want UPGRADE_CHECKSUM_MISMATCH, got %v", e)
	}
}

func TestFetchManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, validManifest)
	}))
	defer srv.Close()
	m, e := FetchManifest(context.Background(), srv.URL)
	if e != nil {
		t.Fatal(e)
	}
	if m.Version != "v0.3.0" {
		t.Errorf("got version %q", m.Version)
	}
}

func TestFetchManifestNotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, e := FetchManifest(context.Background(), srv.URL)
	if e == nil || e.Code != output.CodeUpgradeCheckFailed {
		t.Fatalf("want UPGRADE_CHECK_FAILED, got %v", e)
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "barista")
	newBin := filepath.Join(dir, ".barista-upgrade-new")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBin, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e := ReplaceBinary(target, newBin); e != nil {
		t.Fatal(e)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Errorf("target not replaced: %q, %v", data, err)
	}
	old := target + ".old"
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(old); err != nil {
			t.Error("windows replace must rename the old binary aside to .old")
		}
	} else {
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Error("unix replace must not leave an .old file")
		}
		fi, _ := os.Stat(target)
		if fi.Mode().Perm() != 0o755 {
			t.Errorf("unix replace must chmod 0755, got %v", fi.Mode().Perm())
		}
	}
}

func TestCleanupStale(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "barista")
	if err := os.WriteFile(exe+".old", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	CleanupStale(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Error(".old file should be removed")
	}
}
