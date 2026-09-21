package gradle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"barista/internal/output"
)

const versionsAllFixture = `[
  {"version":"1.0-milestone-3","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/gradle-1.0-milestone-3-bin.zip","checksumUrl":"https://x/gradle-1.0-milestone-3-bin.zip.sha256","checksum":"0","current":false,"final":false},
  {"version":"9.0.0-rc-1","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"9.0.0","milestoneFor":"","downloadUrl":"https://x/gradle-9.0.0-rc-1-bin.zip","checksumUrl":"https://x/gradle-9.0.0-rc-1-bin.zip.sha256","checksum":"a","current":false,"final":false},
  {"version":"8.10.1","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/gradle-8.10.1-bin.zip","checksumUrl":"https://x/gradle-8.10.1-bin.zip.sha256","checksum":"b","current":false,"final":true},
  {"version":"8.5","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/gradle-8.5-bin.zip","checksumUrl":"https://x/gradle-8.5-bin.zip.sha256","checksum":"c","current":false,"final":true},
  {"version":"9.1.0-20260921000000+0000","snapshot":true,"nightly":true,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/nightly.zip","checksumUrl":"","checksum":"d","current":false,"final":false},
  {"version":"7.6.4","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/gradle-7.6.4-bin.zip","checksumUrl":"https://x/gradle-7.6.4-bin.zip.sha256","checksum":"e","current":false,"final":true},
  {"version":"9.0.0-milestone-7","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"9.0.0","downloadUrl":"https://x/gradle-9.0.0-milestone-7-bin.zip","checksumUrl":"https://x/gradle-9.0.0-milestone-7-bin.zip.sha256","checksum":"f","current":false,"final":false},
  {"version":"8.11.0","snapshot":false,"nightly":false,"releaseNightly":false,"broken":true,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/gradle-8.11.0-bin.zip","checksumUrl":"","checksum":"g","current":false,"final":false},
  {"version":"8.10.2","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":"https://x/gradle-8.10.2-bin.zip","checksumUrl":"https://x/gradle-8.10.2-bin.zip.sha256","checksum":"h","current":true,"final":true}
]`

func useVersionsBase(t *testing.T, url string) {
	t.Helper()
	orig := VersionsBase
	VersionsBase = url
	t.Cleanup(func() { VersionsBase = orig })
}

func availableServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/versions/all" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(versionsAllFixture))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func availableFixture(t *testing.T) []AvailableVersion {
	t.Helper()
	srv := availableServer(t)
	useVersionsBase(t, srv.URL)
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
	want := []string{"1.0-milestone-3", "7.6.4", "8.5", "8.10.1", "8.10.2", "9.0.0-milestone-7", "9.0.0-rc-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("versions = %v, want %v", got, want)
	}
	for i, v := range versions {
		if wantLatest := v.Version == "8.10.2"; v.Latest != wantLatest {
			t.Errorf("versions[%d] = %s: Latest = %v, want %v", i, v.Version, v.Latest, wantLatest)
		}
		if wantPre := strings.HasPrefix(v.Version, "9.0.0-") || v.Version == "1.0-milestone-3"; v.Prerelease != wantPre {
			t.Errorf("versions[%d] = %s: Prerelease = %v, want %v", i, v.Version, v.Prerelease, wantPre)
		}
	}
	if versions[4].Checksum != "h" || versions[4].DownloadURL == "" || versions[4].ChecksumURL == "" {
		t.Errorf("8.10.2 entry lost fields: %+v", versions[4])
	}
}

func TestAvailableServerError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	useVersionsBase(t, srv.URL)

	if _, e := Available(context.Background()); e == nil || e.Code != output.CodeGradleAvailableFailed {
		t.Errorf("want GRADLE_AVAILABLE_FAILED, got %v", e)
	}
}

func TestMatchAvailable(t *testing.T) {
	versions := availableFixture(t)
	cases := map[string]string{
		"8.10.2":     "8.10.2", // exact
		"8.5":        "8.5",    // exact
		"8.10":       "8.10.2", // minor prefix
		"8":          "8.10.2", // major prefix
		"8.10.3":     "8.10.2", // missing patch falls back to latest of the line
		"9.0.0-rc-1": "9.0.0-rc-1",
		"9.0.0-rc-2": "9.0.0-rc-1",
		"9":          "9.0.0-rc-1", // prereleases participate in matching
		"7":          "7.6.4",
		"6":          "", // no 6.x in fixture
		"bad":        "",
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

func TestLatestPerMinor(t *testing.T) {
	vs := []AvailableVersion{
		{Version: "8.5"},
		{Version: "8.10.1"},
		{Version: "8.10.2"},
		{Version: "9.0.0-milestone-7"},
		{Version: "9.0.0-rc-1"},
	}
	got := LatestPerMinor(vs)
	var versions []string
	for _, v := range got {
		versions = append(versions, v.Version)
	}
	want := []string{"8.5", "8.10.2", "9.0.0-rc-1"}
	if strings.Join(versions, ",") != strings.Join(want, ",") {
		t.Errorf("LatestPerMinor = %v, want %v", versions, want)
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
	if got := join(RecentMajors(versions, 1)); got != "8.5,8.10.1,8.10.2" {
		t.Errorf("RecentMajors(n=1) = %v", got)
	}
	if got := join(RecentMajors(versions, 2)); got != "7.6.4,8.5,8.10.1,8.10.2" {
		t.Errorf("RecentMajors(n=2) = %v", got)
	}
	if got := join(RecentMajors(versions, 5)); got != "7.6.4,8.5,8.10.1,8.10.2" {
		t.Errorf("RecentMajors(n=5) = %v", got)
	}
	if got := RecentMajors(versions, 0); got != nil {
		t.Errorf("RecentMajors(n=0) = %v, want nil", got)
	}
}
