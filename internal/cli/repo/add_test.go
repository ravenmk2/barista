package repocli

import "testing"

func TestNameFromURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:org/order-service.git": "order-service",
		"https://github.com/org/app.git":       "app",
		"https://github.com/org/app":           "app",
		"https://github.com/org/app/":          "app",
		"file:///srv/git/mono.git":             "mono",
		"order-service":                        "order-service",
		"https://github.com/org/":              "org",
		"":                                     "",
	}
	for in, want := range cases {
		if got := nameFromURL(in); got != want {
			t.Errorf("nameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
