package maven

import (
	"testing"

	"barista/internal/output"
)

func resolveFixture() *Registry {
	return &Registry{
		Installations: []Entry{
			{Name: "maven-3.8.9", Version: "3.8.9", Path: "/a"},
			{Name: "maven-3.9.9", Version: "3.9.9", Path: "/b"},
			{Name: "maven-3.9.11", Version: "3.9.11", Path: "/c"},
			{Name: "maven-4.0.0-rc-4", Version: "4.0.0-rc-4", Path: "/d"},
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
		{"maven-3.9.9", "maven-3.9.9", "name", ""},
		{"3.9.9", "maven-3.9.9", "version", ""},
		{"3.9.11", "maven-3.9.11", "version", ""},
		{"3.9.1", "maven-3.9.11", "prefix", ""},
		{"3.9", "maven-3.9.11", "prefix", ""},
		{"3", "maven-3.9.11", "prefix", ""},
		{"3.8", "maven-3.8.9", "prefix", ""},
		{"3.7", "maven-3.9.11", "prefix", ""},
		{"4", "maven-4.0.0-rc-4", "prefix", ""},
		{"4.0.0-rc-4", "maven-4.0.0-rc-4", "version", ""},
		{"5", "", "", output.CodeMavenNotFound},
		{"nope", "", "", output.CodeMavenNotFound},
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
	reg.Installations = append(reg.Installations, Entry{Name: "3", Version: "2.0.4", Path: "/e"})
	e, source, err := reg.Resolve("3")
	if err != nil || e.Name != "3" || source != "name" {
		t.Errorf("Resolve(\"3\") = %v, %q, %v; want exact name match", e, source, err)
	}
}

func TestResolveDefault(t *testing.T) {
	reg := resolveFixture()
	if _, _, err := reg.Resolve(""); err == nil || err.Code != output.CodeMavenNotFound {
		t.Errorf("empty default: want MAVEN_NOT_FOUND, got %v", err)
	}
	reg.Default = "maven-3.8.9"
	e, source, err := reg.Resolve("")
	if err != nil || e.Name != "maven-3.8.9" || source != "default" {
		t.Errorf("Resolve(\"\") = %v, %q, %v; want maven-3.8.9, default", e, source, err)
	}
	reg.Default = "ghost"
	if _, _, err := reg.Resolve(""); err == nil || err.Code != output.CodeMavenNotFound {
		t.Errorf("dangling default: want MAVEN_NOT_FOUND, got %v", err)
	}
}
