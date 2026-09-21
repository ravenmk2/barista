package gradle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"barista/internal/download"
	"barista/internal/output"
)

type InstallResult struct {
	Entry            Entry
	RequestedVersion string
}

func ParseSHA256(data string) (string, error) {
	fields := strings.Fields(data)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty sha256 file")
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
	return filepath.Join(home, ".barista", "toolchains", "gradle"), nil
}

func InstallDir(cfgValue string) (string, error) {
	if cfgValue != "" {
		return cfgValue, nil
	}
	return DefaultInstallDir()
}

// Install resolves version against the available list, downloads the
// distribution zip (sha256-verified), extracts it under the managed install
// dir, probes the result and registers it. RequestedVersion in the result is
// set when the requested version was substituted by a best match.
func Install(ctx context.Context, reg *Registry, regPath, version, name string, dlOpts *download.Options) (*InstallResult, *output.ErrInfo) {
	if name != "" && reg.Find(name) != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeGradleExists,
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
			Code:    output.CodeGradleNotFound,
			Message: fmt.Sprintf("no gradle version matching %q on services.gradle.org", version),
			Hint:    "list versions with: barista gradle available --all",
		}
	}
	requested := ""
	if match.Version != version {
		requested = version
		version = match.Version
	}
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
			Code:    output.CodeGradleExists,
			Message: fmt.Sprintf("%s already exists but is not registered", filepath.ToSlash(destDir)),
			Hint:    fmt.Sprintf("delete it or register it with: barista gradle add %s", filepath.ToSlash(destDir)),
		}
	}
	tmp, err := os.CreateTemp("", "barista-gradle-*")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeGradleInstallFailed, Message: err.Error()}
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := download.Download(ctx, match.DownloadURL, tmpPath, dlOpts); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeGradleDownloadFailed, Message: err.Error()}
	}
	wantSum := match.Checksum
	if wantSum == "" {
		sumTmp, err := os.CreateTemp("", "barista-gradle-sha256-*")
		if err != nil {
			return nil, &output.ErrInfo{Code: output.CodeGradleInstallFailed, Message: err.Error()}
		}
		sumPath := sumTmp.Name()
		_ = sumTmp.Close()
		defer func() { _ = os.Remove(sumPath) }()
		if err := download.Download(ctx, match.ChecksumURL, sumPath, nil); err != nil {
			return nil, &output.ErrInfo{Code: output.CodeGradleDownloadFailed, Message: "checksum: " + err.Error()}
		}
		sumData, err := os.ReadFile(sumPath)
		if err != nil {
			return nil, &output.ErrInfo{Code: output.CodeGradleDownloadFailed, Message: err.Error()}
		}
		wantSum, err = ParseSHA256(string(sumData))
		if err != nil {
			return nil, &output.ErrInfo{Code: output.CodeGradleDownloadFailed, Message: err.Error()}
		}
	}
	if err := VerifySHA256(tmpPath, wantSum); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeGradleChecksumMismatch, Message: err.Error()}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeGradleInstallFailed, Message: err.Error()}
	}
	tmpDest, err := os.MkdirTemp(root, ".tmp-"+name+"-*")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeGradleInstallFailed, Message: err.Error()}
	}
	if err := download.Extract(tmpPath, tmpDest); err != nil {
		_ = os.RemoveAll(tmpDest)
		return nil, &output.ErrInfo{Code: output.CodeGradleInstallFailed, Message: err.Error()}
	}
	if err := os.Rename(tmpDest, destDir); err != nil {
		_ = os.RemoveAll(tmpDest)
		return nil, &output.ErrInfo{Code: output.CodeGradleInstallFailed, Message: err.Error()}
	}
	info, e := Probe(destDir)
	if e == nil && info.Version != ProbeVersion(version) {
		e = &output.ErrInfo{
			Code:    output.CodeGradleProbeFailed,
			Message: fmt.Sprintf("downloaded gradle reports version %s, want %s", info.Version, ProbeVersion(version)),
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
	return &InstallResult{Entry: entry, RequestedVersion: requested}, nil
}
