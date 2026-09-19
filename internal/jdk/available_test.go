package jdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrimOpenjdkVersion(t *testing.T) {
	cases := map[string]string{
		"17.0.13+11":    "17.0.13",
		"1.8.0_472-b08": "1.8.0_472",
		"21.0.5+11-LTS": "21.0.5",
		"25":            "25",
	}
	for in, want := range cases {
		if got := TrimOpenjdkVersion(in); got != want {
			t.Errorf("TrimOpenjdkVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemurinAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v3/info/available_releases":
			_, _ = w.Write([]byte(`{
				"available_releases": [25, 17, 8, 21],
				"available_lts_releases": [8, 17, 21, 25],
				"most_recent_lts": 25
			}`))
		case strings.HasPrefix(r.URL.Path, "/v3/info/release_versions"):
			major := r.URL.Query().Get("version")
			versions := map[string]string{
				"[8,9)":   "1.8.0_472-b08",
				"[17,18)": "17.0.13+11",
				"[21,22)": "21.0.5+11",
				"[25,26)": "25.0.1+9",
			}
			v, ok := versions[major]
			if !ok {
				t.Errorf("unexpected version query %q", major)
			}
			_, _ = w.Write([]byte(`{"versions":[{"openjdk_version":"` + v + `"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	base := TemurinAPIBase
	TemurinAPIBase = srv.URL
	defer func() { TemurinAPIBase = base }()

	releases, e := temurinProvider{}.Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	if len(releases) != 4 {
		t.Fatalf("got %d releases, want 4", len(releases))
	}
	for i, want := range []int{8, 17, 21, 25} {
		if releases[i].Major != want {
			t.Errorf("releases[%d].Major = %d, want %d (sorted ascending)", i, releases[i].Major, want)
		}
		if !releases[i].LTS {
			t.Errorf("releases[%d] should be LTS", i)
		}
	}
	if releases[0].Version != "1.8.0_472" {
		t.Errorf("jdk8 version = %q, want 1.8.0_472", releases[0].Version)
	}
	if releases[1].Version != "17.0.13" {
		t.Errorf("jdk17 version = %q, want 17.0.13", releases[1].Version)
	}
	if !releases[3].Latest || releases[0].Latest {
		t.Errorf("Latest should mark only most_recent_lts (25)")
	}
}

func TestTemurinAvailableNetworkError(t *testing.T) {
	base := TemurinAPIBase
	TemurinAPIBase = "http://127.0.0.1:1"
	defer func() { TemurinAPIBase = base }()

	_, e := temurinProvider{}.Available(context.Background())
	if e == nil || e.Code != "JDK_AVAILABLE_FAILED" {
		t.Fatalf("want JDK_AVAILABLE_FAILED, got %v", e)
	}
	if e.Hint == "" {
		t.Error("network failure must carry a hint")
	}
}
