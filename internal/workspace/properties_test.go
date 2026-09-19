package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProperty(t *testing.T) {
	cf := ConfigFile{Properties: map[string]any{
		"jdk":           "17",
		"maven.threads": 4,
	}}
	if v, ok := cf.Property("jdk"); !ok || v != "17" {
		t.Errorf("Property(jdk) = %q, %v; want 17, true", v, ok)
	}
	if _, ok := cf.Property("maven.threads"); ok {
		t.Error("non-string property must report ok=false")
	}
	if _, ok := cf.Property("missing"); ok {
		t.Error("missing property must report ok=false")
	}
	if _, ok := (ConfigFile{}).Property("jdk"); ok {
		t.Error("nil properties map must report ok=false")
	}
}

func TestSetConfigPropertyCreate(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".barista", "config.json")
	if err := SetConfigProperty(p, "jdk", "17"); err != nil {
		t.Fatalf("SetConfigProperty: %v", err)
	}
	cf, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if v, ok := cf.Property("jdk"); !ok || v != "17" {
		t.Errorf("Property(jdk) = %q, %v; want 17, true", v, ok)
	}
}

func TestSetConfigPropertyPreservesUnknownFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	doc := `{
  "parallel": 4,
  "futureField": {"nested": true},
  "properties": {"maven.default": "maven-3.9", "other": [1, 2]}
}
`
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetConfigProperty(p, "jdk", "8"); err != nil {
		t.Fatalf("SetConfigProperty: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := LoadConfigFile(p)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if cf.Parallel != 4 {
		t.Errorf("parallel lost: %d", cf.Parallel)
	}
	if v, _ := cf.Property("jdk"); v != "8" {
		t.Errorf("jdk = %q, want 8", v)
	}
	if v, _ := cf.Property("maven.default"); v != "maven-3.9" {
		t.Errorf("maven.default = %q, want maven-3.9", v)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["futureField"]; !ok {
		t.Error("unknown top-level field futureField was dropped")
	}
	props := raw["properties"].(map[string]any)
	if _, ok := props["other"]; !ok {
		t.Error("unknown properties key was dropped")
	}
}

func TestSetConfigPropertyInvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetConfigProperty(p, "jdk", "8"); err == nil {
		t.Error("want error for invalid existing config.json")
	}
}

func TestUnknownFieldsIgnored(t *testing.T) {
	cf, err := parseConfig([]byte(`{"installDir":"relative/dir","mavenInstallDir":"also/relative"}`), "test")
	if err != nil {
		t.Fatalf("legacy installDir fields must be tolerated as unknown fields: %v", err)
	}
	if cf.Parallel != 0 || cf.Color != "" || len(cf.Properties) != 0 {
		t.Errorf("unexpected config: %+v", cf)
	}
}
