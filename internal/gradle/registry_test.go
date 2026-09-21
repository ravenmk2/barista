package gradle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"barista/internal/output"
)

func testRegistry() *Registry {
	return &Registry{
		Installations: []Entry{
			{Name: "gradle-8.5", Version: "8.5", Path: "/opt/gradle-8.5"},
			{Name: "gradle-8.10.2", Version: "8.10.2", Path: "/opt/gradle-8.10.2", Managed: true},
		},
		Default: "gradle-8.10.2",
		Jdk:     "17",
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	reg, e := Load(filepath.Join(t.TempDir(), "gradle.json"))
	if e != nil {
		t.Fatalf("Load: %v", e)
	}
	if len(reg.Installations) != 0 || reg.Default != "" || reg.Jdk != "" || reg.InstallDir != "" {
		t.Errorf("want empty registry, got %+v", reg)
	}
}

func TestLoadInstallDir(t *testing.T) {
	abs := "/opt/gradles"
	if runtime.GOOS == "windows" {
		abs = `C:\gradles`
	}
	write := func(doc []byte) string {
		p := filepath.Join(t.TempDir(), "gradle.json")
		if err := os.WriteFile(p, doc, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Run("relative path errors", func(t *testing.T) {
		_, e := Load(write([]byte(`{"installDir":"relative/dir"}`)))
		if e == nil || e.Code != output.CodeConfigError {
			t.Fatalf("want CONFIG_ERROR, got %+v", e)
		}
	})
	t.Run("absolute path ok", func(t *testing.T) {
		doc, _ := json.Marshal(map[string]string{"installDir": abs})
		reg, e := Load(write(doc))
		if e != nil || reg.InstallDir != abs {
			t.Errorf("Load: %+v, %v", reg, e)
		}
	})
	t.Run("tilde expands", func(t *testing.T) {
		reg, e := Load(write([]byte(`{"installDir":"~/gradles"}`)))
		if e != nil {
			t.Fatalf("Load: %v", e)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home dir")
		}
		if want := filepath.Join(home, "gradles"); reg.InstallDir != want {
			t.Errorf("InstallDir = %q, want %q", reg.InstallDir, want)
		}
	})
}

func TestLoadValidation(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{"invalid json", `{broken`},
		{"duplicate name", `{"installations":[{"name":"a","version":"8.10.2","path":"/a"},{"name":"a","version":"8.10.2","path":"/b"}]}`},
		{"missing name", `{"installations":[{"version":"8.10.2","path":"/a"}]}`},
		{"missing version", `{"installations":[{"name":"a","path":"/a"}]}`},
		{"missing path", `{"installations":[{"name":"a","version":"8.10.2"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "gradle.json")
			if err := os.WriteFile(p, []byte(tc.doc), 0o644); err != nil {
				t.Fatal(err)
			}
			reg, e := Load(p)
			if e == nil {
				t.Fatalf("want error, got registry %+v", reg)
			}
			if e.Code != output.CodeConfigError {
				t.Errorf("code = %q, want CONFIG_ERROR", e.Code)
			}
		})
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".barista", "gradle.json")
	reg := testRegistry()
	if e := reg.Save(p); e != nil {
		t.Fatalf("Save: %v", e)
	}
	loaded, e := Load(p)
	if e != nil {
		t.Fatalf("Load: %v", e)
	}
	if len(loaded.Installations) != 2 || loaded.Default != "gradle-8.10.2" || loaded.Jdk != "17" {
		t.Errorf("roundtrip mismatch: %+v", loaded)
	}
	if !loaded.Installations[1].Managed {
		t.Errorf("managed flag lost: %+v", loaded.Installations[1])
	}
}

func TestAddDuplicate(t *testing.T) {
	reg := testRegistry()
	if e := reg.Add(Entry{Name: "gradle-8.5", Version: "8.5", Path: "/x"}); e == nil || e.Code != output.CodeGradleExists {
		t.Errorf("want GRADLE_EXISTS, got %v", e)
	}
	if e := reg.Add(Entry{Name: "gradle-9.0.0", Version: "9.0.0", Path: "/x"}); e != nil {
		t.Errorf("Add: %v", e)
	}
}

func TestRemoveClearsDefault(t *testing.T) {
	reg := testRegistry()
	removed, cleared, e := reg.Remove("gradle-8.10.2")
	if e != nil || removed == nil {
		t.Fatalf("Remove: %v", e)
	}
	if !cleared || reg.Default != "" {
		t.Errorf("default not cleared: cleared=%v default=%q", cleared, reg.Default)
	}
	if reg.Jdk != "17" {
		t.Errorf("jdk binding must survive remove, got %q", reg.Jdk)
	}
	_, cleared, e = reg.Remove("gradle-8.5")
	if e != nil || cleared {
		t.Errorf("non-default remove: cleared=%v, err=%v", cleared, e)
	}
	if _, _, e = reg.Remove("nope"); e == nil || e.Code != output.CodeGradleNotFound {
		t.Errorf("want GRADLE_NOT_FOUND, got %v", e)
	}
}

func TestSetDefault(t *testing.T) {
	reg := testRegistry()
	if _, e := reg.SetDefault("gradle-8.5"); e != nil || reg.Default != "gradle-8.5" {
		t.Errorf("SetDefault: %v, %q", e, reg.Default)
	}
	if _, e := reg.SetDefault("nope"); e == nil || e.Code != output.CodeGradleNotFound {
		t.Errorf("want GRADLE_NOT_FOUND, got %v", e)
	}
}

func TestAvailableName(t *testing.T) {
	reg := testRegistry()
	if got := reg.AvailableName("gradle-9.0.0"); got != "gradle-9.0.0" {
		t.Errorf("AvailableName = %q, want gradle-9.0.0", got)
	}
	reg.Installations = append(reg.Installations, Entry{Name: "gradle-8.10.2-1", Version: "8.10.2", Path: "/y"})
	if got := reg.AvailableName("gradle-8.10.2"); got != "gradle-8.10.2-2" {
		t.Errorf("AvailableName = %q, want gradle-8.10.2-2", got)
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"gradle-8.10": true,
		"g8":          true,
		"a.b_c-d":     true,
		"-bad":        false,
		"BAD":         false,
		"with space":  false,
		"":            false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}
