package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"barista/internal/download"
	"barista/internal/maven"
	"barista/internal/output"
)

const DefaultBaseURL = "https://github.com/ravenmk2/barista/releases/latest/download"

type Asset struct {
	File   string `json:"file"`
	Format string `json:"format"`
	Entry  string `json:"entry"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Version       string           `json:"version"`
	PublishedAt   string           `json:"publishedAt,omitempty"`
	Assets        map[string]Asset `json:"assets"`
}

func (m *Manifest) Asset(goos, goarch string) (Asset, bool) {
	a, ok := m.Assets[goos+"/"+goarch]
	return a, ok
}

// FetchManifest downloads and validates the release manifest from
// baseURL (a directory URL whose manifest.json asset is fetched).
func FetchManifest(ctx context.Context, baseURL string) (*Manifest, *output.ErrInfo) {
	tmp, err := os.CreateTemp("", "barista-manifest-*.json")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: err.Error()}
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_ = tmp.Close()
	url := strings.TrimRight(baseURL, "/") + "/manifest.json"
	if err := download.Download(ctx, url, tmp.Name(), nil); err != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeUpgradeCheckFailed,
			Message: fmt.Sprintf("cannot fetch %s: %v", url, err),
			Hint:    "check your network connection",
		}
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: err.Error()}
	}
	return ParseManifest(data)
}

func ParseManifest(data []byte) (*Manifest, *output.ErrInfo) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: fmt.Sprintf("invalid manifest: %v", err)}
	}
	if m.SchemaVersion != 2 || m.Version == "" || len(m.Assets) == 0 {
		return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: "manifest is missing schemaVersion 2, version or assets"}
	}
	for platform, a := range m.Assets {
		if a.Entry == "" {
			a.Entry = "barista"
			if strings.HasPrefix(platform, "windows/") {
				a.Entry = "barista.exe"
			}
			m.Assets[platform] = a
		}
	}
	return &m, nil
}

// Newer reports whether latest is newer than current. A current version
// that is not a release tag (dev builds, dirty trees) is treated as
// unknown and always upgradeable; known is then false.
func Newer(current, latest string) (newer, known bool) {
	c := strings.TrimPrefix(current, "v")
	l := strings.TrimPrefix(latest, "v")
	if c == "" || c == "dev" || strings.Contains(c, "-") {
		return true, false
	}
	return maven.CompareVersions(l, c) > 0, true
}

// VerifySHA256 checks the file's sha256 against the hex-encoded want.
func VerifySHA256(path, want string) *output.ErrInfo {
	f, err := os.Open(path)
	if err != nil {
		return &output.ErrInfo{Code: output.CodeUpgradeDownloadFailed, Message: err.Error()}
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return &output.ErrInfo{Code: output.CodeUpgradeDownloadFailed, Message: err.Error()}
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return &output.ErrInfo{
			Code:    output.CodeUpgradeChecksumMismatch,
			Message: fmt.Sprintf("sha256 mismatch: got %s, want %s", got, want),
			Hint:    "the download may be corrupted; try again",
		}
	}
	return nil
}

// CleanupStale removes the leftover <exe>.old from a previous Windows
// self-upgrade. Best-effort.
func CleanupStale(exe string) {
	_ = os.Remove(exe + ".old")
}

// ReplaceBinary atomically replaces target with the file at newBin.
// Windows cannot overwrite a running executable, so the old one is
// renamed aside first; unix renames over the target directly.
func ReplaceBinary(target, newBin string) *output.ErrInfo {
	fail := func(err error) *output.ErrInfo {
		return &output.ErrInfo{
			Code:    output.CodeUpgradeReplaceFailed,
			Message: fmt.Sprintf("cannot replace %s: %v", filepath.ToSlash(target), err),
			Hint:    "make sure the install location is writable, or replace the binary manually",
		}
	}
	if runtime.GOOS == "windows" {
		old := target + ".old"
		_ = os.Remove(old)
		if err := os.Rename(target, old); err != nil {
			return fail(err)
		}
		if err := os.Rename(newBin, target); err != nil {
			_ = os.Rename(old, target)
			return fail(err)
		}
		return nil
	}
	if err := os.Chmod(newBin, 0o755); err != nil {
		return fail(err)
	}
	if err := os.Rename(newBin, target); err != nil {
		return fail(err)
	}
	return nil
}
