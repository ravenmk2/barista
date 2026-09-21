package gradlecli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWrapper(t *testing.T, dir, content string) {
	t.Helper()
	wrapperDir := filepath.Join(dir, "gradle", "wrapper")
	if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrapperDir, "gradle-wrapper.properties"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWrapperVersion(t *testing.T) {
	t.Run("found via upward walk bounded by git root", func(t *testing.T) {
		root := t.TempDir()
		repo := filepath.Join(root, "repos", "app")
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeWrapper(t, repo, "distributionUrl=https://services.gradle.org/distributions/gradle-8.10.2-bin.zip\n")
		nested := filepath.Join(repo, "module", "src")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		v, f, ok := wrapperVersion(nested, root)
		if !ok || v != "8.10.2" {
			t.Errorf("got (%q, %v), want (8.10.2, true)", v, ok)
		}
		want := filepath.Join(repo, "gradle", "wrapper", "gradle-wrapper.properties")
		if f != want {
			t.Errorf("got file %q, want %q", f, want)
		}
	})

	t.Run("wrapper above the git root is not detected", func(t *testing.T) {
		root := t.TempDir()
		writeWrapper(t, root, "distributionUrl=https://services.gradle.org/distributions/gradle-8.10.2-bin.zip\n")
		repo := filepath.Join(root, "repos", "app")
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := wrapperVersion(repo, root); ok {
			t.Error("wrapper above the checkout root must not be detected")
		}
	})

	t.Run("no git root falls back to workspace root boundary", func(t *testing.T) {
		root := t.TempDir()
		writeWrapper(t, root, "distributionUrl=https://services.gradle.org/distributions/gradle-8.10.2-bin.zip\n")
		nested := filepath.Join(root, "repos", "app")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := wrapperVersion(nested, root); !ok {
			t.Error("wrapper at the workspace root must be found")
		}
	})

	t.Run("no properties file", func(t *testing.T) {
		root := t.TempDir()
		if _, _, ok := wrapperVersion(root, ""); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("wrapper dir without properties file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "gradle", "wrapper"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := wrapperVersion(root, ""); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("unparseable distributionUrl", func(t *testing.T) {
		root := t.TempDir()
		writeWrapper(t, root, "distributionUrl=https://example.com/gradle.zip\n")
		if _, _, ok := wrapperVersion(root, ""); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("outside a workspace only cwd is checked", func(t *testing.T) {
		parent := t.TempDir()
		writeWrapper(t, parent, "distributionUrl=https://services.gradle.org/distributions/gradle-8.10.2-bin.zip\n")
		child := filepath.Join(parent, "sub")
		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := wrapperVersion(child, ""); ok {
			t.Error("wrapper in a parent of cwd must not be detected outside a workspace/git checkout")
		}
		if v, _, ok := wrapperVersion(parent, ""); !ok || v != "8.10.2" {
			t.Errorf("wrapper at cwd must be found, got (%q, %v)", v, ok)
		}
	})
}
