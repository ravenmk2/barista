package jdkcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/jdk"
	"barista/internal/output"
)

type fakeProvider struct {
	url string
	sum string
	err error
}

func (p fakeProvider) ArchiveURL(int, string, string) (string, error) { return p.url, p.err }
func (p fakeProvider) Available(context.Context) ([]jdk.AvailableRelease, *output.ErrInfo) {
	return nil, nil
}
func (p fakeProvider) ExpectedSHA256(int, string, string) (string, bool) { return p.sum, p.sum != "" }

func runDownloadForTest(t *testing.T, prov jdk.Provider, distro string, major int, goos, goarch, outFlag string, jsonOut bool) (stdout, stderr string, code int) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	exitCode = &code
	cmd := downloadCmd()
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("json", jsonOut, "")
	_, p, ok := userSettings(cmd)
	if !ok {
		t.Fatal("userSettings failed")
	}
	runDownload(cmd, p, prov, distro, major, goos, goarch, outFlag)

	_ = wOut.Close()
	_ = wErr.Close()
	out, _ := io.ReadAll(rOut)
	errOut, _ := io.ReadAll(rErr)
	return string(out), string(errOut), code
}

func TestResolveDestPath(t *testing.T) {
	dir := t.TempDir()
	if got := resolveDestPath("", "jdk.tar.gz"); got != "jdk.tar.gz" {
		t.Errorf("empty output: got %q", got)
	}
	if got := resolveDestPath(dir, "jdk.tar.gz"); got != filepath.Join(dir, "jdk.tar.gz") {
		t.Errorf("existing dir: got %q", got)
	}
	trailing := filepath.Join(dir, "sub") + string(os.PathSeparator)
	if got := resolveDestPath(trailing, "jdk.tar.gz"); got != filepath.Join(dir, "sub", "jdk.tar.gz") {
		t.Errorf("trailing separator: got %q", got)
	}
	file := filepath.Join(dir, "custom-name.bin")
	if got := resolveDestPath(file, "jdk.tar.gz"); got != file {
		t.Errorf("file path: got %q", got)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got := resolveDestPath("~/sub", "jdk.tar.gz"); got != filepath.Join(home, "sub") {
		t.Errorf("tilde expansion: got %q", got)
	}
}

func TestFallbackFileName(t *testing.T) {
	cases := []struct {
		distro   string
		finalURL string
		want     string
	}{
		{"temurin", "https://cdn.example.com/OpenJDK21U-jdk_x64_linux_hotspot_21.0.5_11.tar.gz", "temurin-jdk-21-linux-amd64.tar.gz"},
		{"temurin", "https://cdn.example.com/jdk.zip", "temurin-jdk-21-linux-amd64.zip"},
		{"temurin", "https://cdn.example.com/jdk.tgz", "temurin-jdk-21-linux-amd64.tgz"},
		{"temurin", "https://cdn.example.com/jdk.TAR.GZ", "temurin-jdk-21-linux-amd64.tar.gz"},
		{"temurin", "https://cdn.example.com/jdk.tar.gz?sig=abc", "temurin-jdk-21-linux-amd64.tar.gz"},
		{"temurin", "https://cdn.example.com/binary/latest", "temurin-jdk-21-linux-amd64"},
		{"openjdk", "https://cdn.example.com/jdk.tar.gz", "openjdk-21-linux-amd64.tar.gz"},
	}
	for _, c := range cases {
		if got := fallbackFileName(c.distro, 21, "linux", "amd64", c.finalURL); got != c.want {
			t.Errorf("fallbackFileName(%q, %q) = %q, want %q", c.distro, c.finalURL, got, c.want)
		}
	}
}

func TestRunDownloadHappyPath(t *testing.T) {
	body := "fake jdk archive content"
	sum := sha256.Sum256([]byte(body))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Disposition", `attachment; filename="fake-jdk-99.tar.gz"`)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	prov := fakeProvider{url: srv.URL + "/binary/latest", sum: hex.EncodeToString(sum[:])}
	outDir := t.TempDir()
	stdout, stderr, code := runDownloadForTest(t, prov, "fakejdk", 99, "linux", "amd64", outDir, false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	dest := filepath.Join(outDir, "fake-jdk-99.tar.gz")
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != body {
		t.Fatalf("downloaded file: err=%v content=%q", err, data)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Error(".part file must be renamed away")
	}
	wantLine := "downloaded fakejdk99 (linux/amd64) at " + filepath.ToSlash(dest)
	if !strings.Contains(stdout, wantLine) {
		t.Errorf("stdout missing %q, got %q", wantLine, stdout)
	}
	if !strings.Contains(stderr, "downloading "+prov.url) {
		t.Errorf("stderr missing downloading line, got %q", stderr)
	}
}

func TestRunDownloadJSON(t *testing.T) {
	body := "fake jdk archive content"
	sum := sha256.Sum256([]byte(body))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Disposition", `attachment; filename="fake-jdk-99.zip"`)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	prov := fakeProvider{url: srv.URL + "/binary/latest", sum: hex.EncodeToString(sum[:])}
	outDir := t.TempDir()
	stdout, _, code := runDownloadForTest(t, prov, "fakejdk", 99, "linux", "amd64", outDir, true)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var env struct {
		Results []struct {
			Name   string         `json:"name"`
			Status string         `json:"status"`
			Action string         `json:"action"`
			Path   string         `json:"path"`
			Detail map[string]any `json:"detail"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(env.Results) != 1 {
		t.Fatalf("want 1 result, got %+v", env.Results)
	}
	r := env.Results[0]
	if r.Name != "fakejdk99" || r.Status != "ok" || r.Action != "download" {
		t.Errorf("result = %+v", r)
	}
	dest := filepath.Join(outDir, "fake-jdk-99.zip")
	if r.Path != filepath.ToSlash(dest) {
		t.Errorf("path = %q, want %q", r.Path, filepath.ToSlash(dest))
	}
	wantDetail := map[string]any{
		"distro":    "fakejdk",
		"major":     float64(99),
		"os":        "linux",
		"arch":      "amd64",
		"url":       prov.url,
		"path":      filepath.ToSlash(dest),
		"sizeBytes": float64(len(body)),
		"sha256":    hex.EncodeToString(sum[:]),
	}
	for k, want := range wantDetail {
		if r.Detail[k] != want {
			t.Errorf("detail[%q] = %v, want %v", k, r.Detail[k], want)
		}
	}
}

func TestRunDownloadFallbackFileName(t *testing.T) {
	body := "fake jdk archive"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	prov := fakeProvider{url: srv.URL + "/"}
	outDir := t.TempDir()
	_, stderr, code := runDownloadForTest(t, prov, "fakejdk", 99, "linux", "amd64", outDir, false)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	dest := filepath.Join(outDir, "fakejdk-99-linux-amd64")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("fallback-named file missing: %v", err)
	}
}

func TestRunDownloadExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Disposition", `attachment; filename="fake-jdk-99.tar.gz"`)
			return
		}
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	dest := filepath.Join(outDir, "fake-jdk-99.tar.gz")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	prov := fakeProvider{url: srv.URL}
	_, stderr, code := runDownloadForTest(t, prov, "fakejdk", 99, "linux", "amd64", outDir, false)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "failed fakejdk99: JDK_EXISTS") {
		t.Errorf("stderr missing JDK_EXISTS failure, got %q", stderr)
	}
	if !strings.Contains(stderr, "hint: delete the file or pass a different --output") {
		t.Errorf("stderr missing hint, got %q", stderr)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "old" {
		t.Error("existing file must not be overwritten")
	}
}

func TestRunDownloadChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Disposition", `attachment; filename="fake-jdk-99.tar.gz"`)
			return
		}
		_, _ = w.Write([]byte("tampered body"))
	}))
	defer srv.Close()

	prov := fakeProvider{url: srv.URL, sum: strings.Repeat("0", 64)}
	outDir := t.TempDir()
	_, stderr, code := runDownloadForTest(t, prov, "fakejdk", 99, "linux", "amd64", outDir, false)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "JDK_CHECKSUM_MISMATCH") {
		t.Errorf("stderr missing JDK_CHECKSUM_MISMATCH, got %q", stderr)
	}
	dest := filepath.Join(outDir, "fake-jdk-99.tar.gz")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("dest must not exist after checksum mismatch")
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Error(".part must be deleted after checksum mismatch")
	}
}

func TestRunDownloadUnsupportedPlatform(t *testing.T) {
	prov := fakeProvider{err: os.ErrNotExist}
	_, stderr, code := runDownloadForTest(t, prov, "fakejdk", 99, "linux", "amd64", "", false)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "failed fakejdk99: JDK_UNSUPPORTED_PLATFORM") {
		t.Errorf("stderr missing JDK_UNSUPPORTED_PLATFORM, got %q", stderr)
	}
}

func TestDownloadCmdArgsValidation(t *testing.T) {
	for _, args := range [][]string{
		{"not-a-distro"},
		{"temurin17", "--os", "plan9"},
		{"temurin17", "--arch", "386"},
		{"temurin17", "--attempts", "0"},
		{"temurin17", "extra"},
	} {
		cmd := downloadCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("args %v: want usage error", args)
		}
	}
}
