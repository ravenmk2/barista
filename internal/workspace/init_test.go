package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	root := t.TempDir()
	created, err := Init(root, "git@github.com:org/", "main")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Repos.BaseURL != "git@github.com:org/" || ws.Repos.DefaultBranch != "main" || len(ws.Repos.Repos) != 0 {
		t.Errorf("unexpected skeleton: %+v", ws.Repos)
	}

	created, err = Init(root, "git@github.com:other/", "trunk")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second init must not overwrite repos.json")
	}
	ws, _ = Load(root)
	if ws.Repos.BaseURL != "git@github.com:org/" {
		t.Error("repos.json was overwritten")
	}
}

func TestInitMissingDir(t *testing.T) {
	_, err := Init(filepath.Join(t.TempDir(), "gone"), "", "")
	if err == nil {
		t.Fatal("want error for missing directory")
	}
}

func TestScanCheckouts(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel), ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mk("app")
	mk("repos/svc")
	if err := os.MkdirAll(filepath.Join(root, "deep", "a", "b", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".barista"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hidden", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ScanCheckouts(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app", "repos/svc"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
