package maven

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"barista/internal/output"
)

func availableServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<a href="../">../</a><a href="maven-3/">maven-3/</a><a href="maven-4/">maven-4/</a>`))
		case "/maven-3/":
			_, _ = w.Write([]byte(`<a href="../">../</a><a href="3.8.9/">3.8.9/</a><a href="3.9.0/">3.9.0/</a><a href="3.9.9/">3.9.9/</a><a href="3.9.11/">3.9.11/</a><a href="README.txt">x</a>`))
		case "/maven-4/":
			_, _ = w.Write([]byte(`<a href="4.0.0-rc-4/">4.0.0-rc-4/</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestAvailable(t *testing.T) {
	srv := availableServer(t)
	defer srv.Close()
	orig := ArchiveBase
	ArchiveBase = srv.URL
	defer func() { ArchiveBase = orig }()

	versions, e := Available(context.Background())
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	var got []string
	for _, v := range versions {
		got = append(got, v.Version)
	}
	want := []string{"3.8.9", "3.9.0", "3.9.9", "3.9.11", "4.0.0-rc-4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("versions = %v, want %v", got, want)
	}
	if !versions[len(versions)-1].Latest {
		t.Errorf("newest version should be marked Latest: %+v", versions)
	}
	for _, v := range versions {
		if wantPre := v.Version == "4.0.0-rc-4"; v.Prerelease != wantPre {
			t.Errorf("%s: Prerelease = %v, want %v", v.Version, v.Prerelease, wantPre)
		}
	}
}

func TestAvailableServerError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	orig := ArchiveBase
	ArchiveBase = srv.URL
	defer func() { ArchiveBase = orig }()

	if _, e := Available(context.Background()); e == nil || e.Code != output.CodeMavenAvailableFailed {
		t.Errorf("want MAVEN_AVAILABLE_FAILED, got %v", e)
	}
}

func TestMatchAvailable(t *testing.T) {
	versions, e := func() ([]AvailableVersion, *output.ErrInfo) {
		srv := availableServer(t)
		t.Cleanup(srv.Close)
		orig := ArchiveBase
		ArchiveBase = srv.URL
		t.Cleanup(func() { ArchiveBase = orig })
		return Available(context.Background())
	}()
	if e != nil {
		t.Fatalf("Available: %v", e)
	}
	cases := map[string]string{
		"3.9.11":     "3.9.11", // exact
		"3.9.0":      "3.9.0",  // exact: trailing zero is not widened
		"3.9":        "3.9.11", // minor prefix; must not "exact match" 3.9.0
		"3":          "3.9.11", // major prefix
		"3.9.10":     "3.9.11", // missing patch falls back to latest of the line
		"4.0.0-rc-2": "4.0.0-rc-4",
		"3.8":        "3.8.9",
		"2":          "", // no 2.x in fixture
		"bad":        "",
	}
	for in, want := range cases {
		got, ok := MatchAvailable(in, versions)
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

func TestMatchAvailablePrefersStable(t *testing.T) {
	versions := []AvailableVersion{
		{Version: "4.0.0-rc-4", Prerelease: true},
		{Version: "4.0.0"},
		{Version: "4.1.0-rc-1", Prerelease: true},
	}
	cases := map[string]string{
		"4":          "4.0.0",      // stable beats a newer prerelease at the same prefix depth
		"4.0.0-rc-2": "4.0.0",      // missing rc falls back to the stable on the same line
		"4.1":        "4.1.0-rc-1", // a longer prefix wins over a shorter stable match
		"4.0.0-rc-4": "4.0.0-rc-4", // exact prerelease match still wins
	}
	for in, want := range cases {
		got, ok := MatchAvailable(in, versions)
		if !ok || got.Version != want {
			t.Errorf("MatchAvailable(%q) = %v, %v; want %s", in, got.Version, ok, want)
		}
	}
}

func TestLatestPerMinor(t *testing.T) {
	vs := []AvailableVersion{
		{Version: "3.8.8"},
		{Version: "3.8.9"},
		{Version: "3.9.9"},
		{Version: "3.9.11"},
		{Version: "4.0.0-rc-2"},
		{Version: "4.0.0-rc-4"},
	}
	got := LatestPerMinor(vs)
	var versions []string
	for _, v := range got {
		versions = append(versions, v.Version)
	}
	want := []string{"3.8.9", "3.9.11", "4.0.0-rc-4"}
	if strings.Join(versions, ",") != strings.Join(want, ",") {
		t.Errorf("LatestPerMinor = %v, want %v", versions, want)
	}
}
