package mvncli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrapperVersion(t *testing.T) {
	writeWrapper := func(t *testing.T, basedir, content string) {
		t.Helper()
		dir := filepath.Join(basedir, ".mvn", "wrapper")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "maven-wrapper.properties"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("found via upward .mvn walk", func(t *testing.T) {
		root := t.TempDir()
		writeWrapper(t, root, "distributionUrl=https://example.com/apache-maven-3.9.11-bin.zip\n")
		nested := filepath.Join(root, "module", "src")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		v, f, ok := wrapperVersion(nested, nil)
		if !ok || v != "3.9.11" {
			t.Errorf("got (%q, %v), want (3.9.11, true)", v, ok)
		}
		want := filepath.Join(root, ".mvn", "wrapper", "maven-wrapper.properties")
		if f != want {
			t.Errorf("got file %q, want %q", f, want)
		}
	})

	t.Run("no properties file", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("MAVEN_BASEDIR", root)
		if _, _, ok := wrapperVersion(root, nil); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run(".mvn without wrapper dir", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("MAVEN_BASEDIR", root)
		if err := os.MkdirAll(filepath.Join(root, ".mvn"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := wrapperVersion(root, nil); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("unparseable distributionUrl", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("MAVEN_BASEDIR", root)
		writeWrapper(t, root, "distributionUrl=https://example.com/maven.zip\n")
		if _, _, ok := wrapperVersion(root, nil); ok {
			t.Error("unexpected hit")
		}
	})

	t.Run("-f flag redirects basedir", func(t *testing.T) {
		root := t.TempDir()
		proj := filepath.Join(root, "proj")
		writeWrapper(t, proj, "distributionUrl=https://example.com/apache-maven-4.0.0-bin.zip\n")
		if err := os.WriteFile(filepath.Join(proj, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
			t.Fatal(err)
		}
		v, _, ok := wrapperVersion(root, []string{"-f", filepath.Join("proj", "pom.xml"), "validate"})
		if !ok || v != "4.0.0" {
			t.Errorf("got (%q, %v), want (4.0.0, true)", v, ok)
		}
	})
}
