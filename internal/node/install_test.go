package node

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"barista/internal/output"
)

func TestParseSHASums(t *testing.T) {
	sum := sha256.Sum256([]byte("payload"))
	h := hex.EncodeToString(sum[:])
	data := strings.Repeat("0", 64) + "  node-v22.14.0-linux-x64.tar.gz\n" +
		h + "  node-v22.14.0-win-x64.zip\n"
	got, err := ParseSHASums(data, "node-v22.14.0-win-x64.zip")
	if err != nil || got != h {
		t.Errorf("ParseSHASums = %q, %v; want %q", got, err, h)
	}
	if _, err := ParseSHASums(data, "node-v22.14.0-darwin-arm64.tar.gz"); err == nil {
		t.Error("missing entry: want error")
	}
	if _, err := ParseSHASums("zz  a.zip", "a.zip"); err == nil {
		t.Error("invalid hex: want error")
	}
	if _, err := ParseSHASums(strings.Repeat("a", 63)+"  a.zip", "a.zip"); err == nil {
		t.Error("short hash: want error")
	}
}

func TestVerifySHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	if err := VerifySHA256(p, hex.EncodeToString(sum[:])); err != nil {
		t.Errorf("VerifySHA256 match: %v", err)
	}
	if err := VerifySHA256(p, strings.Repeat("0", 64)); err == nil {
		t.Error("VerifySHA256 mismatch: want error")
	}
}

func TestPlatformSuffix(t *testing.T) {
	cases := map[string]string{
		"windows/amd64": "win-x64.zip",
		"windows/arm64": "win-arm64.zip",
		"linux/amd64":   "linux-x64.tar.gz",
		"linux/arm64":   "linux-arm64.tar.gz",
		"darwin/amd64":  "darwin-x64.tar.gz",
		"darwin/arm64":  "darwin-arm64.tar.gz",
	}
	for in, want := range cases {
		got, e := PlatformSuffix(strings.Split(in, "/")[0], strings.Split(in, "/")[1])
		if e != nil || got != want {
			t.Errorf("PlatformSuffix(%s) = %q, %v; want %q", in, got, e, want)
		}
	}
	if _, e := PlatformSuffix("plan9", "amd64"); e == nil || e.Code != output.CodeNodeUnsupportedPlatform {
		t.Errorf("want NODE_UNSUPPORTED_PLATFORM, got %v", e)
	}
}

func TestInstallDir(t *testing.T) {
	if got, err := InstallDir(`D:\nodes`); err != nil || got != `D:\nodes` {
		t.Errorf("InstallDir(cfg) = %q, %v", got, err)
	}
	got, err := InstallDir("")
	if err != nil {
		t.Fatalf("InstallDir(default): %v", err)
	}
	if !strings.Contains(filepath.ToSlash(got), ".barista/toolchains/node") {
		t.Errorf("default install dir = %q", got)
	}
}

func platformSuffix(t *testing.T) string {
	t.Helper()
	suffix, e := PlatformSuffix(runtime.GOOS, runtime.GOARCH)
	if e != nil {
		t.Skipf("no node distribution for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return suffix
}

func fakeDistribution(t *testing.T, version string) (data []byte, sum string) {
	t.Helper()
	suffix := platformSuffix(t)
	top := "node-v" + version + "-" + strings.TrimSuffix(strings.TrimSuffix(suffix, ".tar.gz"), ".zip")
	var buf bytes.Buffer
	if strings.HasSuffix(suffix, ".zip") {
		zw := zip.NewWriter(&buf)
		name := top + "/node.exe"
		if runtime.GOOS != "windows" {
			name = top + "/bin/node"
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("binary")); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		name := top + "/bin/node"
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len("binary"))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("binary")); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	s := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(s[:])
}

func nodeDistServer(t *testing.T, version string, archive []byte, sum string) *httptest.Server {
	t.Helper()
	suffix := platformSuffix(t)
	asset := "node-v" + version + "-" + suffix
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_, _ = fmt.Fprintf(w, `[{"version":"v%s","date":"2025-02-11","files":[],"lts":"Jod"}]`, version)
		case "/v" + version + "/" + asset:
			_, _ = w.Write(archive)
		case "/v" + version + "/SHASUMS256.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func installFixture(t *testing.T, version string) (*Registry, string) {
	t.Helper()
	archive, sum := fakeDistribution(t, version)
	srv := nodeDistServer(t, version, archive, sum)
	useDistBase(t, srv.URL)
	useExecNodeVersion(t, "v"+version)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "node.json")
	return reg, regPath
}

func TestInstall(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	res, e := Install(context.Background(), reg, regPath, "22.14.0", "", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.RequestedVersion != "" {
		t.Errorf("RequestedVersion = %q, want empty", res.RequestedVersion)
	}
	if res.DownloadURL == "" {
		t.Error("DownloadURL must record the actual download source")
	}
	if res.Entry.Name != "node-22.14.0" || res.Entry.Version != "22.14.0" || !res.Entry.Managed {
		t.Errorf("entry = %+v", res.Entry)
	}
	if _, err := os.Stat(BinaryPath(res.Entry.Path)); err != nil {
		t.Errorf("installed binary: %v", err)
	}
	loaded, e := Load(regPath)
	if e != nil || len(loaded.Installations) != 1 || loaded.Installations[0].Name != "node-22.14.0" {
		t.Errorf("registry after install: %+v, %v", loaded, e)
	}
}

func TestInstallVPrefix(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	res, e := Install(context.Background(), reg, regPath, "v22.14.0", "", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.RequestedVersion != "" || res.Entry.Version != "22.14.0" {
		t.Errorf("result = %+v, want exact v-prefixed install without substitution", res)
	}
}

func TestInstallResolved(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	versions, e := Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	match, found := MatchAvailable(versions, "22")
	if !found {
		t.Fatal("no match for 22")
	}
	suffix := platformSuffix(t)
	downloadURL := DownloadURL(DistBase, match.Version, suffix)
	res, e := InstallResolved(context.Background(), reg, regPath, match, "", downloadURL, "", nil)
	if e != nil {
		t.Fatalf("InstallResolved: %v", e)
	}
	if res.RequestedVersion != "" {
		t.Errorf("RequestedVersion = %q, want empty (substitution is the caller's concern)", res.RequestedVersion)
	}
	if res.DownloadURL != downloadURL {
		t.Errorf("DownloadURL = %q, want %q", res.DownloadURL, downloadURL)
	}
	if res.Entry.Name != "node-22.14.0" || res.Entry.Version != "22.14.0" {
		t.Errorf("entry = %+v", res.Entry)
	}
}

func TestInstallFuzzySubstitution(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	res, e := Install(context.Background(), reg, regPath, "22", "", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.RequestedVersion != "22" || res.Entry.Version != "22.14.0" || res.Entry.Name != "node-22.14.0" {
		t.Errorf("result = %+v", res)
	}
}

func TestInstallExplicitName(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	res, e := Install(context.Background(), reg, regPath, "22.14.0", "my-node", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.Entry.Name != "my-node" || filepath.Base(res.Entry.Path) != "my-node" {
		t.Errorf("entry = %+v", res.Entry)
	}
}

func TestInstallNameConflict(t *testing.T) {
	useDistBase(t, "http://127.0.0.1:1")
	reg := &Registry{InstallDir: t.TempDir()}
	reg.Installations = append(reg.Installations, Entry{Name: "my-node", Version: "20.19.0", Path: "/x"})
	regPath := filepath.Join(t.TempDir(), "node.json")
	if _, e := Install(context.Background(), reg, regPath, "22.14.0", "my-node", "", nil); e == nil || e.Code != output.CodeNodeExists {
		t.Errorf("want NODE_EXISTS before any network, got %v", e)
	}
}

func TestInstallAutoNameConflictSuffix(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	reg.Installations = append(reg.Installations, Entry{Name: "node-22.14.0", Version: "22.14.0", Path: "/x"})
	res, e := Install(context.Background(), reg, regPath, "22.14.0", "", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.Entry.Name != "node-22.14.0-1" {
		t.Errorf("name = %q, want node-22.14.0-1", res.Entry.Name)
	}
}

func TestInstallNoMatch(t *testing.T) {
	reg, regPath := installFixture(t, "22.14.0")
	if _, e := Install(context.Background(), reg, regPath, "23", "", "", nil); e == nil || e.Code != output.CodeNodeNotFound {
		t.Errorf("want NODE_NOT_FOUND, got %v", e)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	archive, _ := fakeDistribution(t, "22.14.0")
	srv := nodeDistServer(t, "22.14.0", archive, strings.Repeat("0", 64))
	useDistBase(t, srv.URL)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "node.json")
	_, e := Install(context.Background(), reg, regPath, "22.14.0", "", "", nil)
	if e == nil || e.Code != output.CodeNodeChecksumMismatch {
		t.Fatalf("want NODE_CHECKSUM_MISMATCH, got %v", e)
	}
	if e.Hint != "" {
		t.Errorf("official-source mismatch must not carry a mirror hint, got %q", e.Hint)
	}
	entries, err := os.ReadDir(reg.InstallDir)
	if err == nil && len(entries) != 0 {
		t.Errorf("install dir not clean after failure: %v", entries)
	}
}

func TestMirrorDownloadURL(t *testing.T) {
	official := "https://nodejs.org/dist/v22.14.0/node-v22.14.0-win-x64.zip"
	if got := MirrorDownloadURL(official, "https://cdn.npmmirror.com/binaries/node"); got != "https://cdn.npmmirror.com/binaries/node/v22.14.0/node-v22.14.0-win-x64.zip" {
		t.Errorf("mirror rewrite = %q", got)
	}
	if got := MirrorDownloadURL(official, ""); got != official {
		t.Errorf("empty base = %q", got)
	}
	if got := MirrorDownloadURL("https://example.com/other.zip", "https://mirrors.example.com/node"); got != "https://example.com/other.zip" {
		t.Errorf("non-dist URL = %q", got)
	}
}

func TestInstallMirrorFallback(t *testing.T) {
	archive, sum := fakeDistribution(t, "22.14.0")
	official := nodeDistServer(t, "22.14.0", archive, sum)
	useDistBase(t, official.URL)
	useExecNodeVersion(t, "v22.14.0")
	mirror := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(mirror.Close)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "node.json")
	res, e := Install(context.Background(), reg, regPath, "22.14.0", "", mirror.URL, nil)
	if e != nil {
		t.Fatalf("Install with mirror fallback: %v", e)
	}
	suffix := platformSuffix(t)
	if want := DownloadURL(official.URL, "22.14.0", suffix); res.DownloadURL != want {
		t.Errorf("DownloadURL = %q, want fallback %q", res.DownloadURL, want)
	}
}

func TestInstallMirrorPrimary(t *testing.T) {
	archive, sum := fakeDistribution(t, "22.14.0")
	official := nodeDistServer(t, "22.14.0", archive, sum)
	useDistBase(t, official.URL)
	useExecNodeVersion(t, "v22.14.0")
	suffix := platformSuffix(t)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(mirror.Close)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "node.json")
	res, e := Install(context.Background(), reg, regPath, "22.14.0", "", mirror.URL, nil)
	if e != nil {
		t.Fatalf("Install with mirror: %v", e)
	}
	if want := mirror.URL + "/v22.14.0/node-v22.14.0-" + suffix; res.DownloadURL != want {
		t.Errorf("DownloadURL = %q, want mirror %q", res.DownloadURL, want)
	}
}

func TestInstallMirrorChecksumMismatchHint(t *testing.T) {
	archive, _ := fakeDistribution(t, "22.14.0")
	official := nodeDistServer(t, "22.14.0", archive, strings.Repeat("0", 64))
	useDistBase(t, official.URL)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(mirror.Close)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "node.json")
	_, e := Install(context.Background(), reg, regPath, "22.14.0", "", mirror.URL, nil)
	if e == nil || e.Code != output.CodeNodeChecksumMismatch {
		t.Fatalf("want NODE_CHECKSUM_MISMATCH, got %v", e)
	}
	if !strings.Contains(e.Hint, "mirror") {
		t.Errorf("mirror mismatch should hint at the mirror configuration, got %q", e.Hint)
	}
}
