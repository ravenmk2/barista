package jdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsJavaHomeEnv(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"JAVA_HOME", true},
		{"JAVA_HOME_11_X64", true},
		{"JAVA8_HOME", true},
		{"JAVA21_HOME", true},
		{"JAVAHOME", false},
		{"JAVA", false},
		{"MAVEN_HOME", false},
		{"PATH", false},
		{"GRAALVM_HOME", false},
	}
	for _, tc := range cases {
		if got := isJavaHomeEnv(tc.key); got != tc.want {
			t.Errorf("isJavaHomeEnv(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"jdk-a", "jdk-b"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := scanDir(dir, "system")
	if len(got) != 2 {
		t.Fatalf("scanDir found %d candidates, want 2 (files skipped)", len(got))
	}
	if got[0].Path != filepath.Join(dir, "jdk-a") || got[0].Source != "system" {
		t.Errorf("unexpected first candidate: %+v", got[0])
	}
	if got := scanDir(filepath.Join(dir, "missing"), "system"); got != nil {
		t.Errorf("scanDir on missing dir = %v, want nil", got)
	}
}

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	if !SamePath(dir, dir+string(filepath.Separator)) {
		t.Error("same dir with trailing separator should match")
	}
	if SamePath(dir, other) {
		t.Error("different dirs should not match")
	}
	if !SamePath(dir, filepath.Join(dir, "..", filepath.Base(dir))) {
		t.Error("unclean path should match after cleaning")
	}
}
