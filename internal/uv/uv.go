package uv

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"barista/internal/download"
	"barista/internal/output"
)

// DefaultBaseURL hosts the official uv release assets; a sibling
// <asset>.sha256 file sits next to every archive.
const DefaultBaseURL = "https://github.com/astral-sh/uv/releases"

// Sources maps named download sources to release base URLs. Both are
// operated by Astral: astral is their Cloudflare CDN (no /latest/download/
// endpoint, so latest must be resolved first), github the canonical release
// hosting.
var Sources = map[string]string{
	"astral": "https://releases.astral.sh/github/uv/releases",
	"github": DefaultBaseURL,
}

// AstralInstallerURL is the cargo-dist installer for the latest uv release;
// it carries a hardcoded APP_VERSION and serves as the astral-side latest
// pointer.
const AstralInstallerURL = "https://releases.astral.sh/installers/uv/latest/uv-installer.sh"

// ValidateSource checks a --source value: empty (default astral), a known
// source name, or a custom https:// base URL.
func ValidateSource(value string) error {
	if value == "" {
		return nil
	}
	if _, ok := Sources[value]; ok {
		return nil
	}
	if strings.HasPrefix(value, "https://") {
		return nil
	}
	return fmt.Errorf("invalid uv download source %q (want astral|github or an https:// base URL)", value)
}

// SourceBase resolves a --source value to the release base URL and whether
// it is the github source (the only one with a /latest/download/ endpoint).
func SourceBase(value string) (base string, isGithub bool, err error) {
	if err := ValidateSource(value); err != nil {
		return "", false, err
	}
	if value == "" {
		value = "astral"
	}
	if base, ok := Sources[value]; ok {
		return base, value == "github", nil
	}
	return strings.TrimRight(value, "/"), false, nil
}

var appVersionRe = regexp.MustCompile(`(?m)^APP_VERSION="([^"]+)"`)

// ResolveLatest fetches the cargo-dist installer script and parses the
// hardcoded APP_VERSION.
func ResolveLatest(ctx context.Context, installerURL string) (string, *output.ErrInfo) {
	tmp, err := os.CreateTemp("", "barista-uv-installer-*")
	if err != nil {
		return "", &output.ErrInfo{Code: output.CodeUvDownloadFailed, Message: err.Error()}
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_ = tmp.Close()
	if err := download.Download(ctx, installerURL, tmp.Name(), nil); err != nil {
		return "", &output.ErrInfo{
			Code:    output.CodeUvDownloadFailed,
			Message: fmt.Sprintf("cannot resolve the latest uv version from %s: %v", installerURL, err),
			Hint:    "check your network connection, or pin a version with --version",
		}
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", &output.ErrInfo{Code: output.CodeUvDownloadFailed, Message: err.Error()}
	}
	m := appVersionRe.FindSubmatch(data)
	if m == nil {
		return "", &output.ErrInfo{
			Code:    output.CodeUvDownloadFailed,
			Message: fmt.Sprintf("no APP_VERSION found in %s", installerURL),
		}
	}
	return string(m[1]), nil
}

type Platform struct {
	Target string // rust target triple used in the asset name
	Ext    string // ".zip" or ".tar.gz"
}

func PlatformFor(goos, goarch string) (Platform, bool) {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
	a, ok := arch[goarch]
	if !ok {
		return Platform{}, false
	}
	switch goos {
	case "windows":
		return Platform{Target: a + "-pc-windows-msvc", Ext: ".zip"}, true
	case "linux":
		return Platform{Target: a + "-unknown-linux-gnu", Ext: ".tar.gz"}, true
	case "darwin":
		return Platform{Target: a + "-apple-darwin", Ext: ".tar.gz"}, true
	}
	return Platform{}, false
}

// AssetURL resolves the archive URL for version ("" or "latest" tracks the
// latest release via a fixed URL, no API call).
func AssetURL(base, version string, p Platform) string {
	name := "uv-" + p.Target + p.Ext
	if version == "" || version == "latest" {
		return strings.TrimRight(base, "/") + "/latest/download/" + name
	}
	return strings.TrimRight(base, "/") + "/download/" + version + "/" + name
}

func ChecksumURL(assetURL string) string {
	return assetURL + ".sha256"
}

// executableNames are the binaries shipped in uv archives (uvw ships on
// windows only); all of them are installed.
var executableNames = map[string]bool{"uv": true, "uvx": true, "uvw": true}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// BinaryName is the platform-specific name of the uv executable.
func BinaryName() string { return exeName("uv") }

func LocalBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

// InPATH reports whether dir is an entry of the PATH environment variable.
func InPATH(dir string) bool {
	want := filepath.Clean(dir)
	if runtime.GOOS == "windows" {
		want = strings.ToLower(want)
	}
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		entry = filepath.Clean(entry)
		if runtime.GOOS == "windows" {
			entry = strings.ToLower(entry)
		}
		if entry == want {
			return true
		}
	}
	return false
}

// ProbeVersion runs <path> --version and parses the "uv X.Y.Z" first line.
func ProbeVersion(path string) (string, error) {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "uv" {
		return "", fmt.Errorf("unexpected version output %q", strings.TrimSpace(string(out)))
	}
	return fields[1], nil
}

func parseSHA256File(data string) (string, error) {
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

func verifySHA256(path, wantHex string) error {
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

type InstallResult struct {
	Version     string
	Dir         string
	Executables []string
	DownloadURL string
	SameVersion bool
}

// Install downloads the uv archive for version (sha256-verified against the
// sibling .sha256 asset), extracts the bundled executables and installs them
// into destDir. A concrete version equal to existingVersion short-circuits
// before any download; with an unresolved version (github latest) the
// downloaded binary is probed and compared instead. probe reports the
// version of an executable path (ProbeVersion in production, injected in
// tests).
func Install(ctx context.Context, base, version, destDir, existingVersion string, probe func(string) (string, error), dlOpts *download.Options) (*InstallResult, *output.ErrInfo) {
	p, ok := PlatformFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return nil, &output.ErrInfo{
			Code:    output.CodeUvUnsupportedPlatform,
			Message: fmt.Sprintf("no uv release for %s/%s", runtime.GOOS, runtime.GOARCH),
		}
	}
	assetURL := AssetURL(base, version, p)
	res := &InstallResult{Version: version, Dir: destDir, DownloadURL: assetURL}
	if version != "" && existingVersion != "" && existingVersion == version {
		res.SameVersion = true
		return res, nil
	}

	tmp, err := os.MkdirTemp("", "barista-uv-*")
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvInstallFailed, Message: err.Error()}
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	archivePath := filepath.Join(tmp, "archive"+p.Ext)
	if err := download.Download(ctx, assetURL, archivePath, dlOpts); err != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeUvDownloadFailed,
			Message: fmt.Sprintf("cannot download %s: %v", assetURL, err),
			Hint:    "check your network connection and try again",
		}
	}
	sumPath := filepath.Join(tmp, "archive.sha256")
	if err := download.Download(ctx, ChecksumURL(assetURL), sumPath, nil); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvDownloadFailed, Message: "checksum: " + err.Error()}
	}
	sumData, err := os.ReadFile(sumPath)
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvDownloadFailed, Message: err.Error()}
	}
	wantSum, err := parseSHA256File(string(sumData))
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvDownloadFailed, Message: err.Error()}
	}
	if err := verifySHA256(archivePath, wantSum); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvChecksumMismatch, Message: err.Error()}
	}

	stage := filepath.Join(tmp, "stage")
	if err := extractExecutables(archivePath, stage); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvInstallFailed, Message: err.Error()}
	}
	newVersion, _ := probe(filepath.Join(stage, exeName("uv")))
	res.Version = newVersion
	if existingVersion != "" && newVersion != "" && existingVersion == newVersion {
		res.SameVersion = true
		return res, nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvInstallFailed, Message: err.Error()}
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeUvInstallFailed, Message: err.Error()}
	}
	for _, e := range entries {
		if err := copyExecutable(filepath.Join(stage, e.Name()), filepath.Join(destDir, e.Name())); err != nil {
			return nil, &output.ErrInfo{Code: output.CodeUvInstallFailed, Message: err.Error()}
		}
		res.Executables = append(res.Executables, e.Name())
	}
	return res, nil
}

func copyExecutable(src, dst string) error {
	tmp := dst + ".tmp"
	if err := func() error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	}(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// extractExecutables writes the uv/uvx/uvw binaries from a uv release
// archive (zip or tar.gz, files at the archive root) into destDir.
func extractExecutables(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot read archive: %v", err)
	}
	_ = f.Close()
	names := map[string]bool{}
	for base := range executableNames {
		names[exeName(base)] = true
	}
	switch {
	case magic[0] == 'P' && magic[1] == 'K':
		return extractZipExecutables(archivePath, destDir, names)
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return extractTarGzExecutables(archivePath, destDir, names)
	default:
		return fmt.Errorf("unknown archive format (not zip or gzip)")
	}
}

func writeExecutable(destDir, name string, r io.Reader) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	w, err := os.OpenFile(filepath.Join(destDir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func extractZipExecutables(archivePath, destDir string, names map[string]bool) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	found := 0
	for _, e := range zr.File {
		if !names[path.Base(e.Name)] {
			continue
		}
		rc, err := e.Open()
		if err != nil {
			return err
		}
		err = writeExecutable(destDir, path.Base(e.Name), rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		found++
	}
	if found == 0 {
		return fmt.Errorf("archive %s contains no uv executables", archivePath)
	}
	return nil
}

func extractTarGzExecutables(archivePath, destDir string, names map[string]bool) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	found := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || !names[path.Base(hdr.Name)] {
			continue
		}
		if err := writeExecutable(destDir, path.Base(hdr.Name), tr); err != nil {
			return err
		}
		found++
	}
	if found == 0 {
		return fmt.Errorf("archive %s contains no uv executables", archivePath)
	}
	return nil
}
