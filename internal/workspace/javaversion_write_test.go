package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJavaVersionFile(t *testing.T) {
	read := func(t *testing.T, dir string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, ".java-version"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	t.Run("creates new file with trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		prev, err := WriteJavaVersionFile(dir, "17.0.13")
		if err != nil {
			t.Fatal(err)
		}
		if prev != "" {
			t.Errorf("new file must report empty previous, got %q", prev)
		}
		if got := read(t, dir); got != "17.0.13\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("overwrites different value and reports previous", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := WriteJavaVersionFile(dir, "11.0.2"); err != nil {
			t.Fatal(err)
		}
		prev, err := WriteJavaVersionFile(dir, "17.0.13")
		if err != nil {
			t.Fatal(err)
		}
		if prev != "11.0.2" {
			t.Errorf("want previous 11.0.2, got %q", prev)
		}
		if got := read(t, dir); got != "17.0.13\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("same value is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := WriteJavaVersionFile(dir, "17.0.13"); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, ".java-version")
		fi1, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		prev, err := WriteJavaVersionFile(dir, "17.0.13")
		if err != nil {
			t.Fatal(err)
		}
		if prev != "17.0.13" {
			t.Errorf("want previous echoed, got %q", prev)
		}
		fi2, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !fi2.ModTime().Equal(fi1.ModTime()) {
			t.Error("unchanged value must not rewrite the file")
		}
	})

	t.Run("no temp files left behind", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := WriteJavaVersionFile(dir, "21.0.5"); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != ".java-version" {
			names := []string{}
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("want only .java-version, got %v", names)
		}
	})

	t.Run("read error is reported", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".java-version"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := WriteJavaVersionFile(dir, "17"); err == nil {
			t.Fatal("want error when .java-version is a directory")
		}
	})
}
