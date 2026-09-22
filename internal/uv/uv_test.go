package uv

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"barista/internal/output"
)

func TestPlatformFor(t *testing.T) {
	cases := []struct {
		goos, goarch, target, ext string
	}{
		{"windows", "amd64", "x86_64-pc-windows-msvc", ".zip"},
		{"windows", "arm64", "aarch64-pc-windows-msvc", ".zip"},
		{"linux", "amd64", "x86_64-unknown-linux-gnu", ".tar.gz"},
		{"linux", "arm64", "aarch64-unknown-linux-gnu", ".tar.gz"},
		{"darwin", "amd64", "x86_64-apple-darwin", ".tar.gz"},
		{"darwin", "arm64", "aarch64-apple-darwin", ".tar.gz"},
	}
	for _, c := range cases {
		p, ok := PlatformFor(c.goos, c.goarch)
		if !ok || p.Target != c.target || p.Ext != c.ext {
			t.Errorf("PlatformFor(%s, %s) = %+v, %v", c.goos, c.goarch, p, ok)
		}
	}
	if _, ok := PlatformFor("plan9", "amd64"); ok {
		t.Error("plan9 should be unsupported")
	}
	if _, ok := PlatformFor("linux", "386"); ok {
		t.Error("linux/386 should be unsupported")
	}
}

func TestAssetURL(t *testing.T) {
	p := Platform{Target: "x86_64-pc-windows-msvc", Ext: ".zip"}
	got := AssetURL(DefaultBaseURL, "", p)
	want := "https://github.com/astral-sh/uv/releases/latest/download/uv-x86_64-pc-windows-msvc.zip"
	if got != want {
		t.Errorf("latest: got %q, want %q", got, want)
	}
	got = AssetURL(DefaultBaseURL, "0.12.17", p)
	want = "https://github.com/astral-sh/uv/releases/download/0.12.17/uv-x86_64-pc-windows-msvc.zip"
	if got != want {
		t.Errorf("pinned: got %q, want %q", got, want)
	}
	if got := ChecksumURL(want); !strings.HasSuffix(got, ".zip.sha256") {
		t.Errorf("checksum url = %q", got)
	}
}

func TestParseSHA256File(t *testing.T) {
	sum := sha256.Sum256([]byte("payload"))
	h := fmt.Sprintf("%x", sum)
	got, err := parseSHA256File(h + "  uv-x86_64-pc-windows-msvc.zip\n")
	if err != nil || got != h {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "xyz  file", strings.Repeat("g", 64) + "  file"} {
		if _, err := parseSHA256File(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestInPATH(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", filepath.Dir(dir)+string(os.PathListSeparator)+dir)
	if !InPATH(dir) {
		t.Error("dir on PATH should be found")
	}
	if InPATH(filepath.Join(dir, "elsewhere")) {
		t.Error("unrelated dir should not be found")
	}
}

func buildArchive(t *testing.T, ext string, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "archive"+ext)
	w, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if ext == ".zip" {
		zw := zip.NewWriter(w)
		for name, body := range entries {
			f, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		for name, body := range entries {
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractExecutables(t *testing.T) {
	entries := map[string]string{
		exeName("uv"):   "uv-binary",
		exeName("uvx"):  "uvx-binary",
		"README.md":     "ignored",
		"docs/note.txt": "ignored",
	}
	for _, ext := range []string{".zip", ".tar.gz"} {
		t.Run(ext, func(t *testing.T) {
			archive := buildArchive(t, ext, entries)
			dest := t.TempDir()
			if err := extractExecutables(archive, dest); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{exeName("uv"), exeName("uvx")} {
				data, err := os.ReadFile(filepath.Join(dest, name))
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				want := strings.TrimSuffix(name, filepath.Ext(name)) + "-binary"
				if string(data) != want {
					t.Errorf("%s = %q, want %q", name, data, want)
				}
			}
			if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
				t.Error("non-executable entries must be ignored")
			}
		})
	}
}

func TestExtractExecutablesEmpty(t *testing.T) {
	archive := buildArchive(t, ".zip", map[string]string{"README.md": "x"})
	if err := extractExecutables(archive, t.TempDir()); err == nil {
		t.Error("archive without executables should fail")
	}
}

func TestInstall(t *testing.T) {
	entries := map[string]string{exeName("uv"): "uv-binary", exeName("uvx"): "uvx-binary"}
	pf, _ := PlatformFor(runtime.GOOS, runtime.GOARCH)
	ext := pf.Ext
	archive := buildArchive(t, ext, entries)
	archiveData, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archiveData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Base(r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, name)
		case strings.HasSuffix(r.URL.Path, ext):
			_, _ = w.Write(archiveData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fakeProbe := func(string) (string, error) { return "0.12.17", nil }
	dest := t.TempDir()

	res, e := Install(context.Background(), srv.URL, "", dest, "0.12.16", fakeProbe, nil)
	if e != nil {
		t.Fatal(e)
	}
	if res.SameVersion || res.Version != "0.12.17" {
		t.Errorf("got %+v", res)
	}
	for _, name := range []string{exeName("uv"), exeName("uvx")} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Errorf("%s not installed: %v", name, err)
		}
	}

	res, e = Install(context.Background(), srv.URL, "", dest, "0.12.17", fakeProbe, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !res.SameVersion {
		t.Error("same version should be a no-op")
	}
}

func TestValidateSource(t *testing.T) {
	for _, ok := range []string{"", "astral", "github", "https://mirror.example.com/uv"} {
		if err := ValidateSource(ok); err != nil {
			t.Errorf("ValidateSource(%q) should pass: %v", ok, err)
		}
	}
	for _, bad := range []string{"tuna", "http://insecure.example.com"} {
		if err := ValidateSource(bad); err == nil {
			t.Errorf("ValidateSource(%q) should fail", bad)
		}
	}
}

func TestSourceBase(t *testing.T) {
	base, isGithub, err := SourceBase("")
	if err != nil || base != Sources["astral"] || isGithub {
		t.Errorf("default: %q %v %v", base, isGithub, err)
	}
	base, isGithub, err = SourceBase("github")
	if err != nil || base != DefaultBaseURL || !isGithub {
		t.Errorf("github: %q %v %v", base, isGithub, err)
	}
	base, _, err = SourceBase("https://mirror.example.com/uv/")
	if err != nil || base != "https://mirror.example.com/uv" {
		t.Errorf("custom: %q %v", base, err)
	}
}

func TestResolveLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "#!/bin/sh\nAPP_VERSION=\"0.12.17\"\n")
	}))
	defer srv.Close()
	v, e := ResolveLatest(context.Background(), srv.URL)
	if e != nil || v != "0.12.17" {
		t.Fatalf("got %q, %v", v, e)
	}
}

func TestResolveLatestNoVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "#!/bin/sh\necho hello\n")
	}))
	defer srv.Close()
	if _, e := ResolveLatest(context.Background(), srv.URL); e == nil || e.Code != output.CodeUvDownloadFailed {
		t.Fatalf("want UV_DOWNLOAD_FAILED, got %v", e)
	}
}

func TestInstallFastPathSkip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request should hit the network on a same-version install")
	}))
	defer srv.Close()
	res, e := Install(context.Background(), srv.URL, "0.12.17", t.TempDir(), "0.12.17", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !res.SameVersion || res.Version != "0.12.17" {
		t.Errorf("got %+v", res)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	pf, _ := PlatformFor(runtime.GOOS, runtime.GOARCH)
	ext := pf.Ext
	archive := buildArchive(t, ext, map[string]string{exeName("uv"): "uv-binary"})
	archiveData, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			_, _ = fmt.Fprintf(w, "%064x  x\n", 0)
			return
		}
		_, _ = w.Write(archiveData)
	}))
	defer srv.Close()

	_, e := Install(context.Background(), srv.URL, "", t.TempDir(), "", func(string) (string, error) { return "", nil }, nil)
	if e == nil || e.Code != output.CodeUvChecksumMismatch {
		t.Fatalf("want UV_CHECKSUM_MISMATCH, got %v", e)
	}
}
