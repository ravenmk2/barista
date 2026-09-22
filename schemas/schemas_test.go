package schemas

import (
	"encoding/json"
	"os"
	"testing"
)

func TestList(t *testing.T) {
	entries := List()
	if len(entries) != 8 {
		t.Fatalf("List() returned %d entries, want 8", len(entries))
	}
	want := []string{"config", "gradle", "jdk", "manifest", "maven", "node", "properties", "repos"}
	for i, name := range want {
		if entries[i].Name != name {
			t.Fatalf("List()[%d].Name = %q, want %q (sorted)", i, entries[i].Name, name)
		}
	}
	for _, e := range entries {
		if e.Description == "" {
			t.Errorf("entry %q has empty description", e.Name)
		}
		if len(e.Raw) == 0 {
			t.Errorf("entry %q has empty schema", e.Name)
		}
	}
}

func TestGet(t *testing.T) {
	for _, name := range []string{"repos", "config", "jdk", "maven", "gradle", "node", "properties", "manifest"} {
		e, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		disk, err := os.ReadFile(name + ".schema.json")
		if err != nil {
			t.Fatalf("read disk schema: %v", err)
		}
		if string(e.Raw) != string(disk) {
			t.Errorf("Get(%q) embedded content differs from disk file", name)
		}
	}
	if _, err := Get("nope"); err == nil {
		t.Error("Get(\"nope\"): want error, got nil")
	}
}

func TestEmbeddedSchemasAreValidAndCompile(t *testing.T) {
	for _, e := range List() {
		var doc map[string]any
		if err := json.Unmarshal(e.Raw, &doc); err != nil {
			t.Fatalf("schema %q is not valid JSON: %v", e.Name, err)
		}
		if _, ok := doc["$id"]; !ok {
			t.Errorf("schema %q missing $id", e.Name)
		}
		if _, err := validator(e.Name); err != nil {
			t.Errorf("schema %q does not compile: %v", e.Name, err)
		}
	}
}
