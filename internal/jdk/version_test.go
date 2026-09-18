package jdk

import (
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		major   int
		segs    []int
		wantErr bool
	}{
		{"1.8.0_321", 8, []int{8, 0, 321}, false},
		{`"1.8.0_321"`, 8, []int{8, 0, 321}, false},
		{"1.8", 8, []int{8}, false},
		{"9.0.4", 9, []int{9, 0, 4}, false},
		{"11.0.20.1", 11, []int{11, 0, 20, 1}, false},
		{"17.0.7+7", 17, []int{17, 0, 7}, false},
		{"21", 21, []int{21}, false},
		{"21-ea", 21, []int{21}, false},
		{"17.0.7", 17, []int{17, 0, 7}, false},
		{"", 0, nil, true},
		{"abc", 0, nil, true},
		{"-ea", 0, nil, true},
	}
	for _, tc := range cases {
		major, segs, err := ParseVersion(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q): want error, got %d %v", tc.in, major, segs)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", tc.in, err)
			continue
		}
		if major != tc.major || !reflect.DeepEqual(segs, tc.segs) {
			t.Errorf("ParseVersion(%q) = %d %v, want %d %v", tc.in, major, segs, tc.major, tc.segs)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.8.0_291", "1.8.0_321", -1},
		{"1.8.0_321", "1.8.0_291", 1},
		{"17.0.10", "17.0.7", 1},
		{"17.0.7", "17.0.7", 0},
		{"9", "9.0.0", 0},
		{"1.8.0_321", "11.0.1", -1},
		{"21-ea", "21", 0},
		{"junk", "8", -1},
		{"8", "junk", 1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
