package maven

import (
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in        string
		segs      []int
		qualifier string
		wantErr   bool
	}{
		{"3.9.11", []int{3, 9, 11}, "", false},
		{"4.0.0-rc-4", []int{4, 0, 0}, "rc-4", false},
		{"3.8", []int{3, 8}, "", false},
		{"3", []int{3}, "", false},
		{`"3.9.11"`, []int{3, 9, 11}, "", false},
		{"maven", nil, "", true},
		{"3.x", nil, "", true},
		{"", nil, "", true},
		{"3.9.11-", nil, "", true},
	}
	for _, tc := range cases {
		segs, q, err := ParseVersion(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q): want error", tc.in)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(segs, tc.segs) || q != tc.qualifier {
			t.Errorf("ParseVersion(%q) = %v, %q, %v; want %v, %q", tc.in, segs, q, err, tc.segs, tc.qualifier)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"3.9.11", "3.9.9", 1},
		{"3.9.9", "3.9.11", -1},
		{"3.10.0", "3.9.11", 1},
		{"3.9", "3.9.0", 0},
		{"3.9.11", "3.9.11", 0},
		{"4.0.0-rc-4", "4.0.0", -1},
		{"4.0.0", "4.0.0-rc-4", 1},
		{"4.0.0-rc-2", "4.0.0-rc-10", -1},
		{"4.0.0-alpha-1", "4.0.0-beta-1", -1},
		{"4.0.0-rc-4", "4.0.0-rc-4", 0},
		{"4.0.0-rc", "4.0.0-rc-1", -1},
		{"bad", "3.9.11", -1},
		{"3.9.11", "bad", 1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestTwoSegment(t *testing.T) {
	cases := map[string]string{
		"3.9.11":     "3.9",
		"4.0.0-rc-4": "4.0",
		"3.8":        "3.8",
		"3":          "3",
		"bad":        "",
	}
	for in, want := range cases {
		if got := TwoSegment(in); got != want {
			t.Errorf("TwoSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeVersion(t *testing.T) {
	cases := map[string]bool{
		"3":          true,
		"3.9":        true,
		"3.9.11":     true,
		"4.0.0-rc-4": true,
		"maven-3.9":  false,
		"temurin":    false,
		"":           false,
		"3.9.x":      false,
	}
	for in, want := range cases {
		if got := LooksLikeVersion(in); got != want {
			t.Errorf("LooksLikeVersion(%q) = %v, want %v", in, got, want)
		}
	}
}
