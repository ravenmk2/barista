package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"barista/internal/download"
	"barista/internal/output"
)

type InstallResult struct {
	Entry            Entry
	RequestedVersion string
	DownloadURL      string
}

// PlatformSuffix returns the nodejs.org dist asset suffix for the platform
// (windows/amd64 -> win-x64.zip, linux/arm64 -> linux-arm64.tar.gz, ...).
func PlatformSuffix(goos, goarch string) (string, *output.ErrInfo) {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "win-x64.zip", nil
	case "windows/arm64":
		return "win-arm64.zip", nil
	case "linux/amd64":
		return "linux-x64.tar.gz", nil
	case "linux/arm64":
		return "linux-arm64.tar.gz", nil
	case "darwin/amd64":
		return "darwin-x64.tar.gz", nil
	case "darwin/arm64":
		return "darwin-arm64.tar.gz", nil
	}
	return "", &output.ErrInfo{
		Code:    output.CodeNodeUnsupportedPlatform,
		Message: fmt.Sprintf("no node distribution for %s/%s", goos, goarch),
	}
}

// DownloadURL builds the official asset URL for a version and platform suffix.
func DownloadURL(base, version, suffix string) string {
	return fmt.Sprintf("%s/v%s/node-v%s-%s", base, version, version, suffix)
}

// ChecksumURL builds the official SHASUMS256.txt URL for a version.
func ChecksumURL(base, version string) string {
	return fmt.Sprintf("%s/v%s/SHASUMS256.txt", base, version)
}

// ParseSHASums returns the sha256 hex recorded for filename in
// SHASUMS256.txt content ("<hex>  <filename>" per line).
func ParseSHASums(data, filename string) (string, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != filename {
			continue
		}
		h := strings.ToLower(fields[0])
		if len(h) != 64 {
			return "", fmt.Errorf("invalid sha256 %q (want 64 hex chars)", fields[0])
		}
		if _, err := hex.DecodeString(h); err != nil {
			return "", fmt.Errorf("invalid sha256 %q: %v", fields[0], err)
		}
		return h, nil
	}
	return "", fmt.Errorf("no sha256 entry for %s", filename)
}

func VerifySHA256(path, wantHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(wantHex) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, wantHex)
	}
	return nil
}

func DefaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".barista", "toolchains", "node"), nil
}

func InstallDir(cfgValue string) (string, error) {
	if cfgValue != "" {
		return cfgValue, nil
	}
	return DefaultInstallDir()
}

// Install resolves version against the available list (exact match wins,
// otherwise the best prefix match per MatchAvailable) and installs the match.
// RequestedVersion in the result is set when the requested version was
// substituted by a best match. When mirrorBase is non-empty, the asset
// download uses the mirror URL with the official URL as fallback; the version
// listing always comes from the official dist server.
func Install(ctx context.Context, reg *Registry, regPath, version, name, mirrorBase string, dlOpts *download.Options) (*InstallResult, *output.ErrInfo) {
	if name != "" && reg.Find(name) != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeNodeExists,
			Message: fmt.Sprintf("name %q is already registered", name),
		}
	}
	versions, e := Available(ctx)
	if e != nil {
		return nil, e
	}
	match, found := MatchAvailable(versions, version)
	if !found {
		return nil, &output.ErrInfo{
			Code:    output.CodeNodeNotFound,
			Message: fmt.Sprintf("no node version matching %q on nodejs.org", version),
			Hint:    "list versions with: barista node available --all",
		}
	}
	suffix, e := PlatformSuffix(runtime.GOOS, runtime.GOARCH)
	if e != nil {
		return nil, e
	}
	downloadURL := DownloadURL(DistBase, match.Version, suffix)
	fallbackURL := ""
	if mirrorBase != "" {
		fallbackURL = downloadURL
		downloadURL = MirrorDownloadURL(downloadURL, mirrorBase)
	}
	res, e := InstallResolved(ctx, reg, regPath, match, name, downloadURL, fallbackURL, dlOpts)
	if e != nil {
		return nil, e
	}
	if match.Version != strings.TrimPrefix(version, "v") {
		res.RequestedVersion = version
	}
	return res, nil
}

// InstallResolved downloads the matched distribution (sha256-verified against
// the official SHASUMS256.txt), extracts it under the managed install dir,
// probes the result and registers it. Resolution happens beforehand (see
// MatchAvailable), so callers can surface a substitution before any download
// starts. When fallbackURL is non-empty, a failed download of downloadURL
// (e.g. a mirror missing the version) is retried once from fallbackURL;
// InstallResult.DownloadURL records the source actually used.
func InstallResolved(ctx context.Context, reg *Registry, regPath string, match AvailableVersion, name, downloadURL, fallbackURL string, dlOpts *download.Options) (*InstallResult, *output.ErrInfo) {
	if name != "" && reg.Find(name) != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeNodeExists,
			Message: fmt.Sprintf("name %q is already registered", name),
		}
	}
	version := match.Version
	if name == "" {
		name = reg.AvailableName(NameFor(version))
	}
	root, err := InstallDir(reg.InstallDir)
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	destDir := filepath.Join(root, name)
	if _, err := os.Stat(destDir); err == nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeNodeExists,
			Message: fmt.Sprintf("%s already exists but is not registered", filepath.ToSlash(destDir)),
			Hint:    fmt.Sprintf("delete it or register it with: barista node add %s", filepath.ToSlash(destDir)),
		}
	}
	tmp, err := os.CreateTemp("", "barista-node-*")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeInstallFailed, Message: err.Error()}
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	usedURL := downloadURL
	if fallbackURL != "" {
		usedURL, err = download.WithFallback(ctx, downloadURL, fallbackURL, tmpPath, dlOpts)
	} else {
		err = download.Download(ctx, downloadURL, tmpPath, dlOpts)
	}
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeDownloadFailed, Message: err.Error()}
	}
	sumTmp, err := os.CreateTemp("", "barista-node-shasums-*")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeInstallFailed, Message: err.Error()}
	}
	sumPath := sumTmp.Name()
	_ = sumTmp.Close()
	defer func() { _ = os.Remove(sumPath) }()
	if err := download.Download(ctx, ChecksumURL(DistBase, version), sumPath, nil); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeDownloadFailed, Message: "checksum: " + err.Error()}
	}
	sumData, err := os.ReadFile(sumPath)
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeDownloadFailed, Message: err.Error()}
	}
	wantSum, err := ParseSHASums(string(sumData), path.Base(downloadURL))
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeDownloadFailed, Message: err.Error()}
	}
	if err := VerifySHA256(tmpPath, wantSum); err != nil {
		e := &output.ErrInfo{Code: output.CodeNodeChecksumMismatch, Message: err.Error()}
		if fallbackURL != "" {
			e.Hint = "the bytes came from a mirror; check your mirror configuration"
		}
		return nil, e
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeInstallFailed, Message: err.Error()}
	}
	tmpDest, err := os.MkdirTemp(root, ".tmp-"+name+"-*")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeNodeInstallFailed, Message: err.Error()}
	}
	if err := download.Extract(tmpPath, tmpDest); err != nil {
		_ = os.RemoveAll(tmpDest)
		return nil, &output.ErrInfo{Code: output.CodeNodeInstallFailed, Message: err.Error()}
	}
	if err := os.Rename(tmpDest, destDir); err != nil {
		_ = os.RemoveAll(tmpDest)
		return nil, &output.ErrInfo{Code: output.CodeNodeInstallFailed, Message: err.Error()}
	}
	info, e := Probe(destDir)
	if e == nil && info.Version != version {
		e = &output.ErrInfo{
			Code:    output.CodeNodeProbeFailed,
			Message: fmt.Sprintf("downloaded node reports version %s, want %s", info.Version, version),
		}
	}
	if e != nil {
		_ = os.RemoveAll(destDir)
		return nil, e
	}
	entry := Entry{Name: name, Version: version, Path: info.Home, Managed: true}
	if e := reg.Add(entry); e != nil {
		_ = os.RemoveAll(destDir)
		return nil, e
	}
	if e := reg.Save(regPath); e != nil {
		return nil, e
	}
	return &InstallResult{Entry: entry, DownloadURL: usedURL}, nil
}
