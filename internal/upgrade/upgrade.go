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
	Kind      string            `json:"kind"`
	Platforms []string          `json:"platforms,omitempty"`
	File      string            `json:"file"`
	Entry     string            `json:"entry,omitempty"`
	URL       string            `json:"url,omitempty"`
	Hashes    map[string]string `json:"hashes"`
	Size      int64             `json:"size"`
}

type Manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	Version       string  `json:"version"`
	Commit        string  `json:"commit"`
	PublishedAt   string  `json:"publishedAt,omitempty"`
	Assets        []Asset `json:"assets"`
}

// ExecutableAsset returns the executable archive for goos/goarch: the unique
// asset whose kind starts with "executable-" and whose platforms contain the
// pair. Zero matches yield false; multiple matches are a broken manifest.
func (m *Manifest) ExecutableAsset(goos, goarch string) (Asset, bool) {
	platform := goos + "/" + goarch
	var found []Asset
	for _, a := range m.Assets {
		if !strings.HasPrefix(a.Kind, "executable-") {
			continue
		}
		for _, p := range a.Platforms {
			if p == platform {
				found = append(found, a)
				break
			}
		}
	}
	if len(found) != 1 {
		return Asset{}, false
	}
	return found[0], true
}

// ArchiveFormat derives the extractor format from the asset kind
// (executable-zip → zip, executable-tgz → tar.gz).
func (a Asset) ArchiveFormat() string {
	switch strings.TrimPrefix(a.Kind, "executable-") {
	case "zip":
		return "zip"
	case "tgz":
		return "tar.gz"
	}
	return ""
}

// DownloadURL resolves where to fetch the asset: the absolute url field when
// set (mirror copies), otherwise baseURL + file.
func (a Asset) DownloadURL(baseURL string) string {
	if a.URL != "" {
		return a.URL
	}
	return strings.TrimRight(baseURL, "/") + "/" + a.File
}

// SHA256 returns the required sha256 hash of the asset.
func (a Asset) SHA256() string { return a.Hashes["sha256"] }

// FetchManifest downloads and validates the release manifest from
// baseURL (a directory URL whose release.json asset is fetched).
func FetchManifest(ctx context.Context, baseURL string) (*Manifest, *output.ErrInfo) {
	tmp, err := os.CreateTemp("", "barista-manifest-*.json")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: err.Error()}
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_ = tmp.Close()
	url := strings.TrimRight(baseURL, "/") + "/release.json"
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
	if m.SchemaVersion != 2 || m.Version == "" || m.Commit == "" || len(m.Assets) == 0 {
		return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: "manifest is missing schemaVersion 2, version, commit or assets"}
	}
	for i := range m.Assets {
		a := &m.Assets[i]
		if !strings.HasPrefix(a.Kind, "executable-") {
			continue
		}
		if a.Entry == "" {
			a.Entry = "barista"
			for _, p := range a.Platforms {
				if strings.HasPrefix(p, "windows/") {
					a.Entry = "barista.exe"
					break
				}
			}
		}
		if a.Hashes["sha256"] == "" {
			return nil, &output.ErrInfo{Code: output.CodeUpgradeCheckFailed, Message: fmt.Sprintf("asset %s is missing hashes.sha256", a.File)}
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
