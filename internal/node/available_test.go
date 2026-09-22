package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"barista/internal/output"
)

const indexFixture = `[
  {"version":"v24.5.0","date":"2026-09-01","files":["win-x64-zip"],"lts":false},
  {"version":"v22.14.0","date":"2025-02-11","files":["win-x64-zip"],"lts":"Jod"},
  {"version":"v22.13.1","date":"2025-01-21","files":["win-x64-zip"],"lts":"Jod"},
  {"version":"v20.19.0","date":"2025-03-13","files":["win-x64-zip"],"lts":"Iron"},
  {"version":"v20.18.3","date":"2025-02-10","files":["win-x64-zip"],"lts":"Iron"},
  {"version":"not-a-version","date":"2025-01-01","files":[],"lts":false}
]`

func useDistBase(t *testing.T, url string) {
	t.Helper()
	orig := DistBase
	DistBase = url
	t.Cleanup(func() { DistBase = orig })
}

func availableServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(indexFixture))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func availableFixture(t *testing.T) []AvailableVersion {
	t.Helper()
	srv := availableServer(t)
	useDistBase(t, srv.URL)
	versions, e := Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	return versions
}

func TestAvailable(t *testing.T) {
	versions := availableFixture(t)
	var got []string
	for _, v := range versions {
		got = append(got, v.Version)
	}
	want := []string{"24.5.0", "22.14.0", "22.13.1", "20.19.0", "20.18.3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("versions = %v, want %v (descending, unparseable skipped)", got, want)
	}
	for i, v := range versions {
		if wantLatest := v.Version == "24.5.0"; v.Latest != wantLatest {
			t.Errorf("versions[%d] = %s: Latest = %v, want %v", i, v.Version, v.Latest, wantLatest)
		}
	}
	if versions[1].LTS != "Jod" || versions[3].LTS != "Iron" || versions[0].LTS != "" {
		t.Errorf("LTS codenames lost: %+v", versions)
	}
}

func TestAvailableServerError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	useDistBase(t, srv.URL)

	if _, e := Available(context.Background()); e == nil || e.Code != output.CodeNodeAvailableFailed {
		t.Errorf("want NODE_AVAILABLE_FAILED, got %v", e)
	}
}

func TestMatchAvailable(t *testing.T) {
	versions := availableFixture(t)
	cases := map[string]string{
		"22.14.0":  "22.14.0", // exact
		"v22.14.0": "22.14.0", // v prefix accepted
		"22.14":    "22.14.0", // minor prefix
		"22":       "22.14.0", // major prefix
		"22.13.2":  "22.13.1", // missing patch falls back to latest of the line
		"20":       "20.19.0",
		"24":       "24.5.0",
		"23":       "", // no 23.x in fixture
		"bad":      "",
	}
	for in, want := range cases {
		got, ok := MatchAvailable(versions, in)
		if want == "" {
			if ok {
				t.Errorf("MatchAvailable(%q) = %v, want no match", in, got.Version)
			}
			continue
		}
		if !ok || got.Version != want {
			t.Errorf("MatchAvailable(%q) = %v, %v; want %s", in, got.Version, ok, want)
		}
	}
}

func TestLatestPerMajor(t *testing.T) {
	versions := availableFixture(t)
	got := LatestPerMajor(versions)
	var vs []string
	for _, v := range got {
		vs = append(vs, v.Version)
	}
	want := []string{"24.5.0", "22.14.0", "20.19.0"}
	if strings.Join(vs, ",") != strings.Join(want, ",") {
		t.Errorf("LatestPerMajor = %v, want %v", vs, want)
	}
}

func TestRecentMajors(t *testing.T) {
	versions := availableFixture(t)
	join := func(vs []AvailableVersion) string {
		var out []string
		for _, v := range vs {
			out = append(out, v.Version)
		}
		return strings.Join(out, ",")
	}
	if got := join(RecentMajors(versions, 1)); got != "24.5.0" {
		t.Errorf("RecentMajors(n=1) = %v", got)
	}
	if got := join(RecentMajors(versions, 2)); got != "24.5.0,22.14.0,22.13.1" {
		t.Errorf("RecentMajors(n=2) = %v", got)
	}
	if got := join(RecentMajors(versions, 5)); got != "24.5.0,22.14.0,22.13.1,20.19.0,20.18.3" {
		t.Errorf("RecentMajors(n=5) = %v", got)
	}
	if got := RecentMajors(versions, 0); got != nil {
		t.Errorf("RecentMajors(n=0) = %v, want nil", got)
	}
}
