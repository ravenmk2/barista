package gradle

import (
	"testing"

	"barista/internal/output"
)

func resolveFixture() *Registry {
	return &Registry{
		Installations: []Entry{
			{Name: "gradle-8.5", Version: "8.5", Path: "/a"},
			{Name: "gradle-8.10.1", Version: "8.10.1", Path: "/b"},
			{Name: "gradle-8.10.2", Version: "8.10.2", Path: "/c"},
			{Name: "gradle-9.0.0-rc-1", Version: "9.0.0-rc-1", Path: "/d"},
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
		{"gradle-8.10.1", "gradle-8.10.1", "name", ""},
		{"8.10.1", "gradle-8.10.1", "version", ""},
		{"8.10.2", "gradle-8.10.2", "version", ""},
		{"8.10.0", "gradle-8.10.2", "prefix", ""},
		{"8.10", "gradle-8.10.2", "prefix", ""},
		{"8", "gradle-8.10.2", "prefix", ""},
		{"8.5", "gradle-8.5", "version", ""},
		{"8.9", "gradle-8.10.2", "prefix", ""},
		{"9", "gradle-9.0.0-rc-1", "prefix", ""},
		{"9.0.0-rc-1", "gradle-9.0.0-rc-1", "version", ""},
		{"10", "", "", output.CodeGradleNotFound},
		{"nope", "", "", output.CodeGradleNotFound},
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
	reg.Installations = append(reg.Installations, Entry{Name: "8", Version: "7.6.4", Path: "/e"})
	e, source, err := reg.Resolve("8")
	if err != nil || e.Name != "8" || source != "name" {
		t.Errorf("Resolve(\"8\") = %v, %q, %v; want exact name match", e, source, err)
	}
}

func TestResolveDefault(t *testing.T) {
	reg := resolveFixture()
	if _, _, err := reg.Resolve(""); err == nil || err.Code != output.CodeGradleNotFound {
		t.Errorf("empty default: want GRADLE_NOT_FOUND, got %v", err)
	}
	reg.Default = "gradle-8.5"
	e, source, err := reg.Resolve("")
	if err != nil || e.Name != "gradle-8.5" || source != "default" {
		t.Errorf("Resolve(\"\") = %v, %q, %v; want gradle-8.5, default", e, source, err)
	}
	reg.Default = "ghost"
	if _, _, err := reg.Resolve(""); err == nil || err.Code != output.CodeGradleNotFound {
		t.Errorf("dangling default: want GRADLE_NOT_FOUND, got %v", err)
	}
}
