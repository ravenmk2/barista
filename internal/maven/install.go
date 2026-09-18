package maven

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ArchiveBase = "https://archive.apache.org/dist/maven"

func ArchiveURL(version string) (string, error) {
	segs, _, err := ParseVersion(version)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/maven-%d/%s/binaries/apache-maven-%s-bin.tar.gz", ArchiveBase, segs[0], version, version), nil
}

func ChecksumURL(archiveURL string) string {
	return archiveURL + ".sha512"
}

func ParseSHA512(data string) (string, error) {
	fields := strings.Fields(data)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty sha512 file")
	}
	h := strings.ToLower(fields[0])
	if len(h) != 128 {
		return "", fmt.Errorf("invalid sha512 %q (want 128 hex chars)", fields[0])
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", fmt.Errorf("invalid sha512 %q: %v", fields[0], err)
	}
	return h, nil
}

func VerifySHA512(path, wantHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(wantHex) {
		return fmt.Errorf("sha512 mismatch: got %s, want %s", got, wantHex)
	}
	return nil
}

func DefaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".barista", "toolchains", "maven"), nil
}

func InstallDir(cfgValue string) (string, error) {
	if cfgValue != "" {
		return cfgValue, nil
	}
	return DefaultInstallDir()
}
