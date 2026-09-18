package jdk

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseDistroArg(t *testing.T) {
	cases := []struct {
		in     string
		distro string
		major  int
		ok     bool
	}{
		{"temurin17", "temurin", 17, true},
		{"openjdk9", "openjdk", 9, true},
		{"temurin", "", 0, false},
		{"17", "", 0, false},
		{"Temurin17", "", 0, false},
		{"temurin17-lts", "", 0, false},
		{"temurin0", "", 0, false},
	}
	for _, tc := range cases {
		d, m, ok := ParseDistroArg(tc.in)
		if d != tc.distro || m != tc.major || ok != tc.ok {
			t.Errorf("ParseDistroArg(%q) = %q, %d, %v; want %q, %d, %v", tc.in, d, m, ok, tc.distro, tc.major, tc.ok)
		}
	}
}

func TestTemurinArchiveURL(t *testing.T) {
	p := temurinProvider{}
	url, err := p.ArchiveURL(17, "linux", "amd64")
	if err != nil || !strings.Contains(url, "/ga/linux/x64/") {
		t.Errorf("linux/amd64 = %q, %v", url, err)
	}
	url, err = p.ArchiveURL(21, "darwin", "arm64")
	if err != nil || !strings.Contains(url, "/ga/mac/aarch64/") {
		t.Errorf("darwin/arm64 = %q, %v", url, err)
	}
	if _, err := p.ArchiveURL(17, "plan9", "amd64"); err == nil {
		t.Error("unsupported OS should error")
	}
	if _, err := p.ArchiveURL(17, "linux", "386"); err == nil {
		t.Error("unsupported arch should error")
	}
}

func TestDownload(t *testing.T) {
	body := strings.Repeat("barista", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5000")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	var lastReceived, lastTotal int64
	err := Download(context.Background(), srv.URL, dest, &DownloadOptions{
		Backoff: time.Millisecond,
		OnProgress: func(received, total int64) {
			lastReceived, lastTotal = received, total
		},
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != body {
		t.Fatalf("content mismatch: %v", err)
	}
	if lastReceived != int64(len(body)) || lastTotal != 5000 {
		t.Errorf("progress = %d/%d, want %d/5000", lastReceived, lastTotal, len(body))
	}
}

func TestDownloadHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	retries := 0
	err := Download(context.Background(), srv.URL, dest, &DownloadOptions{
		Backoff: time.Millisecond,
		OnRetry: func(int, error) { retries++ },
	})
	if err == nil {
		t.Fatal("want error for 404")
	}
	if retries != 0 {
		t.Errorf("404 must not retry, got %d retries", retries)
	}
}

func TestDownloadRetryResume(t *testing.T) {
	full := strings.Repeat("barista-jdk-archive", 1000)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(full)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(full[:len(full)/2]))
			return
		}
		if !strings.HasPrefix(r.Header.Get("Range"), "bytes=") {
			t.Error("second request should carry a Range header")
		}
		var from int
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from)
		w.Header().Set("Content-Length", strconv.Itoa(len(full)-from))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(full[from:]))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	var retries int
	err := Download(context.Background(), srv.URL, dest, &DownloadOptions{
		Backoff: time.Millisecond,
		OnRetry: func(int, error) { retries++ },
	})
	if err != nil {
		t.Fatalf("Download with resume: %v", err)
	}
	if retries != 1 || calls != 2 {
		t.Errorf("retries = %d, calls = %d; want 1 and 2", retries, calls)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != full {
		t.Errorf("resumed content mismatch (len %d, want %d)", len(data), len(full))
	}
}

func TestDownloadRetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	retries := 0
	err := Download(context.Background(), srv.URL, dest, &DownloadOptions{
		Attempts: 3,
		Backoff:  time.Millisecond,
		OnRetry:  func(int, error) { retries++ },
	})
	if err == nil {
		t.Fatal("want error after exhausting attempts")
	}
	if retries != 2 {
		t.Errorf("retries = %d, want 2 (attempts-1)", retries)
	}
}

func buildZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if content != "" {
			if _, err := w.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractZip(t *testing.T) {
	z := buildZip(t, map[string]string{
		"jdk-17.0.7/":             "",
		"jdk-17.0.7/bin/java.exe": "binary",
		"jdk-17.0.7/release":      "JAVA_VERSION=17",
	})
	dest := filepath.Join(t.TempDir(), "out")
	if err := Extract(z, dest); err != nil {
		t.Fatalf("Extract zip: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "bin", "java.exe"))
	if err != nil || string(data) != "binary" {
		t.Errorf("stripped file = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "release")); err != nil {
		t.Errorf("release missing: %v", err)
	}
}

func TestExtractZipSlip(t *testing.T) {
	z := buildZip(t, map[string]string{"../evil.sh": "x"})
	if err := Extract(z, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("want zip-slip error")
	}
}

func TestExtractTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(hdr *tar.Header, content string) {
		if hdr.Typeflag == tar.TypeReg {
			hdr.Size = int64(len(content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if content != "" {
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(&tar.Header{Name: "jdk-21/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	add(&tar.Header{Name: "jdk-21/bin/java", Typeflag: tar.TypeReg, Mode: 0o755}, "binary")
	add(&tar.Header{Name: "jdk-21/lib/libjli.so", Typeflag: tar.TypeReg, Mode: 0o644}, "lib")
	if runtime.GOOS != "windows" {
		add(&tar.Header{Name: "jdk-21/bin/java.link", Typeflag: tar.TypeSymlink, Linkname: "java"}, "")
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := Extract(p, dest); err != nil {
		t.Fatalf("Extract tar.gz: %v", err)
	}
	javaPath := filepath.Join(dest, "bin", "java")
	data, err := os.ReadFile(javaPath)
	if err != nil || string(data) != "binary" {
		t.Errorf("bin/java = %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(javaPath)
		if fi.Mode().Perm() != 0o755 {
			t.Errorf("java mode = %v, want 0755", fi.Mode().Perm())
		}
		if target, err := os.Readlink(filepath.Join(dest, "bin", "java.link")); err != nil || target != "java" {
			t.Errorf("symlink = %q, %v", target, err)
		}
	}
}

func TestStripFirst(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"jdk-17/bin/java", "bin/java", false},
		{"jdk-17", "", false},
		{"jdk-17/", "", false},
		{"jdk-17/../evil", "", false},
		{"../evil", "", true},
		{"/abs/path", "", true},
	}
	for _, tc := range cases {
		got, err := stripFirst(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("stripFirst(%q): want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("stripFirst(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestInstallDir(t *testing.T) {
	if got, err := InstallDir(`D:\jdks`); err != nil || got != `D:\jdks` {
		t.Errorf("InstallDir(cfg) = %q, %v", got, err)
	}
	got, err := InstallDir("")
	if err != nil {
		t.Fatalf("InstallDir(default): %v", err)
	}
	if !strings.Contains(filepath.ToSlash(got), ".barista/toolchains/jdk") {
		t.Errorf("default install dir = %q", got)
	}
}
