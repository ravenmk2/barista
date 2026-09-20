package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindUpward(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "app")
	deep := filepath.Join(repo, "src", "main")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(repo, ".java-version")
	if err := os.WriteFile(marker, []byte("17\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("hit at repo root from nested dir", func(t *testing.T) {
		p, ok := FindUpward(deep, root, ".java-version")
		if !ok || p != marker {
			t.Errorf("got (%q, %v), want (%q, true)", p, ok, marker)
		}
	})

	t.Run("hit at intermediate level", func(t *testing.T) {
		mid := filepath.Join(root, "mid")
		inner := filepath.Join(mid, "inner")
		if err := os.MkdirAll(inner, 0o755); err != nil {
			t.Fatal(err)
		}
		m := filepath.Join(mid, ".tool-version")
		if err := os.WriteFile(m, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, ok := FindUpward(inner, root, ".tool-version")
		if !ok || p != m {
			t.Errorf("got (%q, %v), want (%q, true)", p, ok, m)
		}
	})

	t.Run("boundary is inclusive", func(t *testing.T) {
		p, ok := FindUpward(deep, repo, ".java-version")
		if !ok || p != marker {
			t.Errorf("got (%q, %v), want (%q, true)", p, ok, marker)
		}
	})

	t.Run("no hit beyond boundary", func(t *testing.T) {
		_, ok := FindUpward(deep, filepath.Join(repo, "src"), ".java-version")
		if ok {
			t.Error("file above boundary must not be found")
		}
	})

	t.Run("empty boundary climbs to filesystem root", func(t *testing.T) {
		p, ok := FindUpward(deep, "", ".java-version")
		if !ok || p != marker {
			t.Errorf("got (%q, %v), want (%q, true)", p, ok, marker)
		}
	})

	t.Run("start outside boundary checks start only", func(t *testing.T) {
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, ".java-version"), []byte("21"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, ok := FindUpward(outside, root, ".java-version")
		if !ok || filepath.Base(p) != ".java-version" {
			t.Errorf("start itself must still be checked, got (%q, %v)", p, ok)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, ok := FindUpward(deep, root, ".nope"); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("directory with target name does not match", func(t *testing.T) {
		dir := filepath.Join(root, "fakedir")
		if err := os.MkdirAll(filepath.Join(dir, ".java-version"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, ok := FindUpward(dir, root, ".java-version"); ok {
			t.Error("directories must not match")
		}
	})
}

func TestFindGitRoot(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "app")
	deep := filepath.Join(repo, "src", "main")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("directory .git hit", func(t *testing.T) {
		d, ok := FindGitRoot(deep, root)
		if !ok || d != repo {
			t.Errorf("got (%q, %v), want (%q, true)", d, ok, repo)
		}
	})

	t.Run("file .git worktree hit", func(t *testing.T) {
		wt := filepath.Join(root, "repos", "wt")
		sub := filepath.Join(wt, "pkg")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: ../app/.git/worktrees/wt\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, ok := FindGitRoot(sub, root)
		if !ok || d != wt {
			t.Errorf("got (%q, %v), want (%q, true)", d, ok, wt)
		}
	})

	t.Run("nearest nested checkout wins", func(t *testing.T) {
		nested := filepath.Join(repo, "modules", "nested")
		if err := os.MkdirAll(filepath.Join(nested, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		d, ok := FindGitRoot(nested, root)
		if !ok || d != nested {
			t.Errorf("got (%q, %v), want (%q, true)", d, ok, nested)
		}
	})

	t.Run("no .git within boundary", func(t *testing.T) {
		island := t.TempDir()
		sub := filepath.Join(island, "a", "b")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, ok := FindGitRoot(sub, island); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("empty boundary climbs to filesystem root", func(t *testing.T) {
		d, ok := FindGitRoot(deep, "")
		if !ok || d != repo {
			t.Errorf("got (%q, %v), want (%q, true)", d, ok, repo)
		}
	})

	t.Run(".git above boundary is not crossed", func(t *testing.T) {
		inner := filepath.Join(repo, "src")
		if _, ok := FindGitRoot(deep, inner); ok {
			t.Error(".git above boundary must not be found")
		}
	})
}
