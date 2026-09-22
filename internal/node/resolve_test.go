package node

import (
	"testing"

	"barista/internal/output"
)

func resolveFixture() *Registry {
	return &Registry{
		Installations: []Entry{
			{Name: "node-20.19.0", Version: "20.19.0", Path: "/a"},
			{Name: "node-22.13.1", Version: "22.13.1", Path: "/b"},
			{Name: "node-22.14.0", Version: "22.14.0", Path: "/c"},
			{Name: "node-24.0.0", Version: "24.0.0", Path: "/d"},
		},
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		arg         string
		wantName    string
		wantSource  string
		wantErrCode string
	}{
		{"node-22.13.1", "node-22.13.1", "name", ""},
		{"22.13.1", "node-22.13.1", "version", ""},
		{"v22.14.0", "node-22.14.0", "version", ""},
		{"22.14", "node-22.14.0", "prefix", ""},
		{"22", "node-22.14.0", "prefix", ""},
		{"v22", "node-22.14.0", "prefix", ""},
		{"20.19.0", "node-20.19.0", "version", ""},
		{"22.15", "node-22.14.0", "prefix", ""},
		{"24", "node-24.0.0", "prefix", ""},
		{"23", "", "", output.CodeNodeNotFound},
		{"nope", "", "", output.CodeNodeNotFound},
	}
	for _, tc := range cases {
		reg := resolveFixture()
		e, source, err := reg.Resolve(tc.arg)
		if tc.wantErrCode != "" {
			if err == nil || err.Code != tc.wantErrCode {
				t.Errorf("Resolve(%q): want %s, got entry=%v err=%v", tc.arg, tc.wantErrCode, e, err)
			}
			continue
		}
		if err != nil || e == nil || e.Name != tc.wantName || source != tc.wantSource {
			t.Errorf("Resolve(%q) = %v, %q, %v; want %q, %q", tc.arg, e, source, err, tc.wantName, tc.wantSource)
		}
	}
}

func TestResolveExactNameBeatsVersion(t *testing.T) {
	reg := resolveFixture()
	reg.Installations = append(reg.Installations, Entry{Name: "22", Version: "20.19.0", Path: "/e"})
	e, source, err := reg.Resolve("22")
	if err != nil || e.Name != "22" || source != "name" {
		t.Errorf("Resolve(\"22\") = %v, %q, %v; want exact name match", e, source, err)
	}
}

func TestResolveDefault(t *testing.T) {
	reg := resolveFixture()
	if _, _, err := reg.Resolve(""); err == nil || err.Code != output.CodeNodeNotFound {
		t.Errorf("empty default: want NODE_NOT_FOUND, got %v", err)
	}
	reg.Default = "node-20.19.0"
	e, source, err := reg.Resolve("")
	if err != nil || e.Name != "node-20.19.0" || source != "default" {
		t.Errorf("Resolve(\"\") = %v, %q, %v; want node-20.19.0, default", e, source, err)
	}
	reg.Default = "ghost"
	if _, _, err := reg.Resolve(""); err == nil || err.Code != output.CodeNodeNotFound {
		t.Errorf("dangling default: want NODE_NOT_FOUND, got %v", err)
	}
}
