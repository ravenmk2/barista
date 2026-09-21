package gradle

import (
	"testing"
)

func TestNameFor(t *testing.T) {
	cases := map[string]string{
		"8.10.2":     "gradle-8.10.2",
		"9.0.0-rc-1": "gradle-9.0.0-rc-1",
		"8.5":        "gradle-8.5",
		"8":          "gradle-8",
		"bad":        "",
	}
	for in, want := range cases {
		if got := NameFor(in); got != want {
			t.Errorf("NameFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeVersion(t *testing.T) {
	cases := map[string]bool{
		"8":           true,
		"8.10":        true,
		"8.10.2":      true,
		"9.0.0-rc-1":  true,
		"gradle-8.10": false,
		"temurin":     false,
		"":            false,
		"8.10.x":      false,
	}
	for in, want := range cases {
		if got := LooksLikeVersion(in); got != want {
			t.Errorf("LooksLikeVersion(%q) = %v, want %v", in, got, want)
		}
	}
}
