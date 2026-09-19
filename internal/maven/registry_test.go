package maven

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
			{Name: "maven-3.8", Version: "3.8.9", Path: "/opt/maven-3.8.9"},
			{Name: "maven-3.9", Version: "3.9.11", Path: "/opt/maven-3.9.11", Managed: true},
		},
		Default: "maven-3.9",
		Jdk:     "17",
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	reg, e := Load(filepath.Join(t.TempDir(), "maven.json"))
	if e != nil {
		t.Fatalf("Load: %v", e)
	}
	if len(reg.Installations) != 0 || reg.Default != "" || reg.Jdk != "" || reg.InstallDir != "" {
		t.Errorf("want empty registry, got %+v", reg)
	}
}

func TestLoadInstallDir(t *testing.T) {
	abs := "/opt/mavens"
	if runtime.GOOS == "windows" {
		abs = `C:\mavens`
	}
	write := func(doc []byte) string {
		p := filepath.Join(t.TempDir(), "maven.json")
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
		reg, e := Load(write([]byte(`{"installDir":"~/mavens"}`)))
		if e != nil {
			t.Fatalf("Load: %v", e)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home dir")
		}
		if want := filepath.Join(home, "mavens"); reg.InstallDir != want {
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
		{"duplicate name", `{"installations":[{"name":"a","version":"3.9.9","path":"/a"},{"name":"a","version":"3.9.9","path":"/b"}]}`},
		{"missing name", `{"installations":[{"version":"3.9.9","path":"/a"}]}`},
		{"missing version", `{"installations":[{"name":"a","path":"/a"}]}`},
		{"missing path", `{"installations":[{"name":"a","version":"3.9.9"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "maven.json")
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
	p := filepath.Join(t.TempDir(), ".barista", "maven.json")
	reg := testRegistry()
	if e := reg.Save(p); e != nil {
		t.Fatalf("Save: %v", e)
	}
	loaded, e := Load(p)
	if e != nil {
		t.Fatalf("Load: %v", e)
	}
	if len(loaded.Installations) != 2 || loaded.Default != "maven-3.9" || loaded.Jdk != "17" {
		t.Errorf("roundtrip mismatch: %+v", loaded)
	}
	if !loaded.Installations[1].Managed {
		t.Errorf("managed flag lost: %+v", loaded.Installations[1])
	}
}

func TestAddDuplicate(t *testing.T) {
	reg := testRegistry()
	if e := reg.Add(Entry{Name: "maven-3.8", Version: "3.8.9", Path: "/x"}); e == nil || e.Code != output.CodeMavenExists {
		t.Errorf("want MAVEN_EXISTS, got %v", e)
	}
	if e := reg.Add(Entry{Name: "maven-4", Version: "4.0.0", Path: "/x"}); e != nil {
		t.Errorf("Add: %v", e)
	}
}

func TestRemoveClearsDefault(t *testing.T) {
	reg := testRegistry()
	removed, cleared, e := reg.Remove("maven-3.9")
	if e != nil || removed == nil {
		t.Fatalf("Remove: %v", e)
	}
	if !cleared || reg.Default != "" {
		t.Errorf("default not cleared: cleared=%v default=%q", cleared, reg.Default)
	}
	if reg.Jdk != "17" {
		t.Errorf("jdk binding must survive remove, got %q", reg.Jdk)
	}
	_, cleared, e = reg.Remove("maven-3.8")
	if e != nil || cleared {
		t.Errorf("non-default remove: cleared=%v, err=%v", cleared, e)
	}
	if _, _, e = reg.Remove("nope"); e == nil || e.Code != output.CodeMavenNotFound {
		t.Errorf("want MAVEN_NOT_FOUND, got %v", e)
	}
}

func TestSetDefault(t *testing.T) {
	reg := testRegistry()
	if _, e := reg.SetDefault("maven-3.8"); e != nil || reg.Default != "maven-3.8" {
		t.Errorf("SetDefault: %v, %q", e, reg.Default)
	}
	if _, e := reg.SetDefault("nope"); e == nil || e.Code != output.CodeMavenNotFound {
		t.Errorf("want MAVEN_NOT_FOUND, got %v", e)
	}
}

func TestAvailableName(t *testing.T) {
	reg := testRegistry()
	if got := reg.AvailableName("maven-4"); got != "maven-4" {
		t.Errorf("AvailableName = %q, want maven-4", got)
	}
	reg.Installations = append(reg.Installations, Entry{Name: "maven-3.9-1", Version: "3.9.9", Path: "/y"})
	if got := reg.AvailableName("maven-3.9"); got != "maven-3.9-2" {
		t.Errorf("AvailableName = %q, want maven-3.9-2", got)
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"maven-3.9":  true,
		"m3":         true,
		"a.b_c-d":    true,
		"-bad":       false,
		"BAD":        false,
		"with space": false,
		"":           false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}
