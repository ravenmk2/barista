package node

import (
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v22.14.0": "22.14.0",
		"22.14.0":  "22.14.0",
		" 22.14.0": "22.14.0",
	} {
		got, err := NormalizeVersion(in)
		if err != nil || got != want {
			t.Errorf("NormalizeVersion(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "v", "node-22", "22.x", "v22.14.0-"} {
		if _, err := NormalizeVersion(bad); err == nil {
			t.Errorf("NormalizeVersion(%q): want error", bad)
		}
	}
}

func TestNameFor(t *testing.T) {
	cases := map[string]string{
		"22.14.0":  "node-22.14.0",
		"v22.14.0": "node-22.14.0",
		"20":       "node-20",
		"bad":      "",
	}
	for in, want := range cases {
		if got := NameFor(in); got != want {
			t.Errorf("NameFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeVersion(t *testing.T) {
	cases := map[string]bool{
		"22":         true,
		"22.14":      true,
		"22.14.0":    true,
		"v22.14.0":   true,
		"node-22.14": false,
		"node":       false,
		"":           false,
		"22.14.x":    false,
	}
	for in, want := range cases {
		if got := LooksLikeVersion(in); got != want {
			t.Errorf("LooksLikeVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseVersionFile(t *testing.T) {
	for in, want := range map[string]string{
		"22.14.0\n":                 "22.14.0",
		"v22.14.0":                  "22.14.0",
		"22.14.0\r\n":               "22.14.0",
		"\n  \n22.14.0\n":           "22.14.0",
		"# comment\nv20.19.0\n":     "20.19.0",
		"# only a comment\n# x\n22": "22",
	} {
		got, ok := ParseVersionFile(in)
		if !ok || got != want {
			t.Errorf("ParseVersionFile(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "\n# nothing\n", "lts/*\n", "not-a-version\n"} {
		if _, ok := ParseVersionFile(bad); ok {
			t.Errorf("ParseVersionFile(%q): want not ok", bad)
		}
	}
}
