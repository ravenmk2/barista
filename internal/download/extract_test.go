package download

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
