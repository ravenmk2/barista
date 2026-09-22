package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"barista/internal/output"
)

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.zip")
	w, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	w, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractEntry(t *testing.T) {
	entries := map[string]string{
		"barista":                  "binary-bytes",
		"LICENSE":                  "license text",
		"completions/barista.bash": "complete -F",
	}
	for _, c := range []struct {
		name    string
		archive string
		format  string
	}{
		{"zip", writeZip(t, entries), "zip"},
		{"tar.gz", writeTarGz(t, entries), "tar.gz"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "barista")
			if e := ExtractEntry(c.archive, c.format, "barista", dest); e != nil {
				t.Fatal(e)
			}
			data, err := os.ReadFile(dest)
			if err != nil || !bytes.Equal(data, []byte("binary-bytes")) {
				t.Errorf("got %q, %v", data, err)
			}
			if runtime.GOOS != "windows" {
				fi, _ := os.Stat(dest)
				if fi.Mode().Perm() != 0o755 {
					t.Errorf("mode = %v, want 0755", fi.Mode().Perm())
				}
			}
		})
	}
}

func TestExtractEntryMissing(t *testing.T) {
	archive := writeZip(t, map[string]string{"LICENSE": "x"})
	dest := filepath.Join(t.TempDir(), "barista")
	e := ExtractEntry(archive, "zip", "barista", dest)
	if e == nil || e.Code != output.CodeUpgradeExtractFailed {
		t.Fatalf("want UPGRADE_EXTRACT_FAILED, got %v", e)
	}
}

func TestExtractEntryUnsupportedFormat(t *testing.T) {
	e := ExtractEntry("whatever", "rar", "barista", filepath.Join(t.TempDir(), "x"))
	if e == nil || e.Code != output.CodeUpgradeExtractFailed || e.Hint == "" {
		t.Fatalf("want UPGRADE_EXTRACT_FAILED with hint, got %v", e)
	}
}
