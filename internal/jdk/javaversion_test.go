package jdk

import "testing"

func TestParseJavaVersionFile(t *testing.T) {
	cases := []struct {
		content string
		major   int
		distro  string
		ok      bool
	}{
		{"17", 17, "", true},
		{"17.0.13", 17, "", true},
		{"1.8", 8, "", true},
		{"temurin-17.0.13", 17, "temurin", true},
		{"17.0.13-tem", 17, "temurin", true},
		{"temurin@17", 17, "temurin", true},
		{"21-zulu", 21, "zulu", true},
		{"ms-21", 21, "microsoft", true},
		{"corretto@11", 11, "corretto", true},
		{"grl-21", 21, "graalvm", true},
		{"  17.0.13\n", 17, "", true},
		{"17\r\n", 17, "", true},
		{"openjdk 17.0.13", 17, "", true},
		{"", 0, "", false},
		{"   \n", 0, "", false},
		{"latest", 0, "", false},
		{"hello world", 0, "", false},
		{"temurin", 0, "", false},
	}
	for _, c := range cases {
		major, distro, ok := ParseJavaVersionFile(c.content)
		if major != c.major || distro != c.distro || ok != c.ok {
			t.Errorf("%q: got (%d, %q, %v), want (%d, %q, %v)", c.content, major, distro, ok, c.major, c.distro, c.ok)
		}
	}
}

func TestResolveJavaVersionSpec(t *testing.T) {
	reg := &Registry{JDKs: []Entry{
		{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/t17"},
		{Name: "zulu17", Major: 17, Version: "17.0.13", Path: "/j/z17"},
	}}

	e, spec := ResolveJavaVersionSpec(reg, 17, "zulu")
	if e == nil || e.Name != "zulu17" || spec != "zulu17" {
		t.Errorf("distro hit: got (%v, %q)", e, spec)
	}

	e, spec = ResolveJavaVersionSpec(reg, 17, "corretto")
	if e == nil || spec != "17" {
		t.Errorf("distro miss must fall back to major resolve: got (%v, %q)", e, spec)
	}

	e, spec = ResolveJavaVersionSpec(reg, 17, "")
	if e == nil || spec != "17" {
		t.Errorf("plain major: got (%v, %q)", e, spec)
	}

	e, spec = ResolveJavaVersionSpec(reg, 21, "")
	if e != nil || spec != "21" {
		t.Errorf("unresolvable major: got (%v, %q)", e, spec)
	}
}
