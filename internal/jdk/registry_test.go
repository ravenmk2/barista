package jdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"barista/internal/output"
)

func sampleRegistry() *Registry {
	return &Registry{
		JDKs: []Entry{
			{Name: "openjdk8", Major: 8, Version: "1.8.0_291", Path: "/jdks/openjdk8"},
			{Name: "temurin8", Major: 8, Version: "1.8.0_321", Path: "/jdks/temurin8"},
			{Name: "temurin17", Major: 17, Version: "17.0.7", Path: "/jdks/temurin17", Managed: true},
		},
		Defaults: map[string]string{"8": "temurin8"},
	}
}

func TestLoadMissing(t *testing.T) {
	reg, e := Load(filepath.Join(t.TempDir(), "jdk.json"))
	if e != nil {
		t.Fatalf("Load missing: %v", e.Message)
	}
	if len(reg.JDKs) != 0 || len(reg.Defaults) != 0 || reg.InstallDir != "" {
		t.Errorf("want empty registry, got %+v", reg)
	}
}

func TestLoadInstallDir(t *testing.T) {
	abs := "/opt/jdks"
	if runtime.GOOS == "windows" {
		abs = `C:\jdks`
	}
	write := func(doc []byte) string {
		p := filepath.Join(t.TempDir(), "jdk.json")
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
		reg, e := Load(write([]byte(`{"installDir":"~/jdks"}`)))
		if e != nil {
			t.Fatalf("Load: %v", e)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home dir")
		}
		if want := filepath.Join(home, "jdks"); reg.InstallDir != want {
			t.Errorf("InstallDir = %q, want %q", reg.InstallDir, want)
		}
	})
}

func TestLoadInvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "jdk.json")
	if err := os.WriteFile(p, []byte(`{ broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, e := Load(p)
	if e == nil || e.Code != output.CodeConfigError {
		t.Fatalf("want CONFIG_ERROR, got %+v", e)
	}
}

func TestLoadDuplicateName(t *testing.T) {
	p := filepath.Join(t.TempDir(), "jdk.json")
	data := `{"jdks":[{"name":"a","major":8,"version":"1.8.0_1","path":"/x"},{"name":"a","major":8,"version":"1.8.0_2","path":"/y"}]}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, e := Load(p)
	if e == nil || e.Code != output.CodeConfigError {
		t.Fatalf("want CONFIG_ERROR, got %+v", e)
	}
}

func TestAddDuplicate(t *testing.T) {
	reg := sampleRegistry()
	e := reg.Add(Entry{Name: "temurin8", Major: 8, Version: "1.8.0_999", Path: "/elsewhere"})
	if e == nil || e.Code != output.CodeJDKExists {
		t.Fatalf("want JDK_EXISTS, got %+v", e)
	}
	if len(reg.JDKs) != 3 {
		t.Errorf("duplicate add mutated registry: %+v", reg.JDKs)
	}
}

func TestRemoveCascadesDefaults(t *testing.T) {
	reg := sampleRegistry()
	removed, cleared, e := reg.Remove("temurin8")
	if e != nil {
		t.Fatalf("Remove: %v", e.Message)
	}
	if removed.Name != "temurin8" {
		t.Errorf("removed = %+v", removed)
	}
	if !reflect.DeepEqual(cleared, []int{8}) {
		t.Errorf("cleared = %v, want [8]", cleared)
	}
	if _, ok := reg.Defaults["8"]; ok {
		t.Errorf("default for 8 should be cleared: %+v", reg.Defaults)
	}
	if len(reg.JDKs) != 2 {
		t.Errorf("jdk count = %d, want 2", len(reg.JDKs))
	}
}

func TestRemoveNotFound(t *testing.T) {
	reg := sampleRegistry()
	_, _, e := reg.Remove("nope")
	if e == nil || e.Code != output.CodeJDKNotFound {
		t.Fatalf("want JDK_NOT_FOUND, got %+v", e)
	}
}

func TestSetDefault(t *testing.T) {
	reg := sampleRegistry()
	if _, e := reg.SetDefault(17, "temurin17"); e != nil {
		t.Fatalf("SetDefault: %v", e.Message)
	}
	if reg.Defaults["17"] != "temurin17" {
		t.Errorf("defaults = %+v", reg.Defaults)
	}
	if _, e := reg.SetDefault(8, "nope"); e == nil || e.Code != output.CodeJDKNotFound {
		t.Fatalf("want JDK_NOT_FOUND, got %+v", e)
	}
	if _, e := reg.SetDefault(11, "temurin8"); e == nil || e.Code != output.CodeJDKMajorMismatch {
		t.Fatalf("want JDK_MAJOR_MISMATCH, got %+v", e)
	}
}

func TestResolve(t *testing.T) {
	reg := sampleRegistry()
	e, source, _ := reg.Resolve("8")
	if e == nil || e.Name != "temurin8" || source != "default" {
		t.Errorf("Resolve(\"8\") = %v, %s; want temurin8 via default", e, source)
	}
	e, source, _ = reg.Resolve("17")
	if e == nil || e.Name != "temurin17" || source != "latest" {
		t.Errorf("Resolve(\"17\") = %v, %s; want temurin17 via latest", e, source)
	}
	e, source, _ = reg.Resolve("temurin8")
	if e == nil || e.Name != "temurin8" || source != "name" {
		t.Errorf("Resolve(\"temurin8\") = %v, %s; want temurin8 via name", e, source)
	}
	if _, _, err := reg.Resolve("21"); err == nil || err.Code != output.CodeJDKNotFound {
		t.Errorf("Resolve(\"21\"): want JDK_NOT_FOUND, got %+v", err)
	}
	if _, _, err := reg.Resolve("ghost"); err == nil || err.Code != output.CodeJDKNotFound {
		t.Errorf("Resolve(\"ghost\"): want JDK_NOT_FOUND, got %+v", err)
	}
}

func TestResolveDanglingDefaultFallsBack(t *testing.T) {
	reg := sampleRegistry()
	reg.Defaults["8"] = "gone"
	e, source, _ := reg.Resolve("8")
	if e == nil || e.Name != "temurin8" || source != "latest" {
		t.Errorf("Resolve(\"8\") with dangling default = %v, %s; want temurin8 via latest", e, source)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "jdk.json")
	reg := sampleRegistry()
	if e := reg.Save(p); e != nil {
		t.Fatalf("Save: %v", e.Message)
	}
	if e := reg.Save(p); e != nil {
		t.Fatalf("Save over existing: %v", e.Message)
	}
	back, e := Load(p)
	if e != nil {
		t.Fatalf("Load: %v", e.Message)
	}
	if !reflect.DeepEqual(back.JDKs, reg.JDKs) || !reflect.DeepEqual(back.Defaults, reg.Defaults) {
		t.Errorf("roundtrip mismatch: %+v vs %+v", back, reg)
	}
}

func TestAvailableName(t *testing.T) {
	reg := sampleRegistry()
	if got := reg.AvailableName("temurin21"); got != "temurin21" {
		t.Errorf("AvailableName free = %q", got)
	}
	if got := reg.AvailableName("temurin8"); got != "temurin8-1" {
		t.Errorf("AvailableName taken = %q, want temurin8-1", got)
	}
	reg.JDKs = append(reg.JDKs, Entry{Name: "temurin8-1", Major: 8, Version: "1.8.0_1", Path: "/z"})
	if got := reg.AvailableName("temurin8"); got != "temurin8-2" {
		t.Errorf("AvailableName twice taken = %q, want temurin8-2", got)
	}
}
