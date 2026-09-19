package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPropertiesString(t *testing.T) {
	p := Properties{"jdk": "17", "threads": float64(4), "offline": true}
	if v, ok := p.String("jdk"); !ok || v != "17" {
		t.Errorf("String(jdk) = %q, %v; want 17, true", v, ok)
	}
	if _, ok := p.String("threads"); ok {
		t.Error("non-string value must report ok=false")
	}
	if _, ok := p.String("missing"); ok {
		t.Error("missing key must report ok=false")
	}
	if _, ok := (Properties{}).String("jdk"); ok {
		t.Error("empty map must report ok=false")
	}
	var nilProps Properties
	if _, ok := nilProps.String("jdk"); ok {
		t.Error("nil map must report ok=false")
	}
}

func TestLoadPropertiesMissing(t *testing.T) {
	p, err := LoadProperties(filepath.Join(t.TempDir(), "properties.json"))
	if err != nil {
		t.Fatalf("LoadProperties missing: %v", err)
	}
	if p == nil || len(p) != 0 {
		t.Errorf("want empty non-nil map, got %v", p)
	}
}

func TestLoadPropertiesScalars(t *testing.T) {
	p := filepath.Join(t.TempDir(), "properties.json")
	if err := os.WriteFile(p, []byte(`{"jdk":"17","threads":4,"offline":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	props, err := LoadProperties(p)
	if err != nil {
		t.Fatalf("LoadProperties: %v", err)
	}
	if v, _ := props.String("jdk"); v != "17" {
		t.Errorf("jdk = %q, want 17", v)
	}
	if props["threads"] != float64(4) || props["offline"] != true {
		t.Errorf("scalar values lost: %v", props)
	}
}

func TestLoadPropertiesValidation(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{"invalid json", `{broken`},
		{"array value", `{"jdk":["17"]}`},
		{"object value", `{"jdk":{"v":1}}`},
		{"null value", `{"jdk":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "properties.json")
			if err := os.WriteFile(p, []byte(tc.doc), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadProperties(p)
			var le *LoadError
			if !errors.As(err, &le) || le.Code != "CONFIG_ERROR" {
				t.Fatalf("want CONFIG_ERROR LoadError, got %v", err)
			}
		})
	}
}

func TestSetProperty(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".barista", "properties.json")
	if err := SetProperty(p, "jdk", "17"); err != nil {
		t.Fatalf("SetProperty create: %v", err)
	}
	props, err := LoadProperties(p)
	if err != nil {
		t.Fatalf("LoadProperties: %v", err)
	}
	if v, _ := props.String("jdk"); v != "17" {
		t.Errorf("jdk = %q, want 17", v)
	}
	if err := SetProperty(p, "maven.default", "maven-3.9"); err != nil {
		t.Fatalf("SetProperty second key: %v", err)
	}
	if err := SetProperty(p, "jdk", "21"); err != nil {
		t.Fatalf("SetProperty overwrite: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["jdk"] != "21" || raw["maven.default"] != "maven-3.9" {
		t.Errorf("overwrite/preservation mismatch: %v", raw)
	}
}

func TestSetPropertyInvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "properties.json")
	if err := os.WriteFile(p, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetProperty(p, "jdk", "8"); err == nil {
		t.Error("want error for invalid existing properties.json")
	}
}

func TestUnknownFieldsIgnored(t *testing.T) {
	cf, err := parseConfig([]byte(`{"installDir":"relative/dir","properties":{"jdk":"17"}}`), "test")
	if err != nil {
		t.Fatalf("legacy config fields must be tolerated as unknown fields: %v", err)
	}
	if cf.Parallel != 0 || cf.Color != "" {
		t.Errorf("unexpected config: %+v", cf)
	}
}
