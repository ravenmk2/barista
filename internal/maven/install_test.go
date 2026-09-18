package maven

import (
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveURL(t *testing.T) {
	url, err := ArchiveURL("3.9.11")
	want := "https://archive.apache.org/dist/maven/maven-3/3.9.11/binaries/apache-maven-3.9.11-bin.tar.gz"
	if err != nil || url != want {
		t.Errorf("ArchiveURL(3.9.11) = %q, %v; want %q", url, err, want)
	}
	url, err = ArchiveURL("4.0.0-rc-4")
	want = "https://archive.apache.org/dist/maven/maven-4/4.0.0-rc-4/binaries/apache-maven-4.0.0-rc-4-bin.tar.gz"
	if err != nil || url != want {
		t.Errorf("ArchiveURL(4.0.0-rc-4) = %q, %v; want %q", url, err, want)
	}
	if _, err = ArchiveURL("bad"); err == nil {
		t.Error("ArchiveURL(bad): want error")
	}
}

func TestChecksumURL(t *testing.T) {
	if got := ChecksumURL("https://x/y.tar.gz"); got != "https://x/y.tar.gz.sha512" {
		t.Errorf("ChecksumURL = %q", got)
	}
}

func TestParseSHA512(t *testing.T) {
	sum := sha512.Sum512([]byte("payload"))
	h := hex.EncodeToString(sum[:])
	if got, err := ParseSHA512(h + "  apache-maven-3.9.11-bin.tar.gz\n"); err != nil || got != h {
		t.Errorf("with filename = %q, %v", got, err)
	}
	if got, err := ParseSHA512(h); err != nil || got != h {
		t.Errorf("bare = %q, %v", got, err)
	}
	if got, err := ParseSHA512(strings.ToUpper(h)); err != nil || got != h {
		t.Errorf("uppercase = %q, %v", got, err)
	}
	for _, bad := range []string{"", "abc", strings.Repeat("g", 128), strings.Repeat("a", 127)} {
		if _, err := ParseSHA512(bad); err == nil {
			t.Errorf("ParseSHA512(%q): want error", bad)
		}
	}
}

func TestVerifySHA512(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512([]byte("payload"))
	if err := VerifySHA512(p, hex.EncodeToString(sum[:])); err != nil {
		t.Errorf("VerifySHA512 match: %v", err)
	}
	if err := VerifySHA512(p, strings.Repeat("0", 128)); err == nil {
		t.Error("VerifySHA512 mismatch: want error")
	}
}

func TestInstallDir(t *testing.T) {
	if got, err := InstallDir(`D:\mavens`); err != nil || got != `D:\mavens` {
		t.Errorf("InstallDir(cfg) = %q, %v", got, err)
	}
	got, err := InstallDir("")
	if err != nil {
		t.Fatalf("InstallDir(default): %v", err)
	}
	if !strings.Contains(filepath.ToSlash(got), ".barista/toolchains/maven") {
		t.Errorf("default install dir = %q", got)
	}
}
