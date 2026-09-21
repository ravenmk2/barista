package gradle

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/output"
)

func TestParseSHA256(t *testing.T) {
	sum := sha256.Sum256([]byte("payload"))
	h := hex.EncodeToString(sum[:])
	if got, err := ParseSHA256(h + "  gradle-8.10.2-bin.zip\n"); err != nil || got != h {
		t.Errorf("with filename = %q, %v", got, err)
	}
	if got, err := ParseSHA256(h); err != nil || got != h {
		t.Errorf("bare = %q, %v", got, err)
	}
	if got, err := ParseSHA256(strings.ToUpper(h)); err != nil || got != h {
		t.Errorf("uppercase = %q, %v", got, err)
	}
	for _, bad := range []string{"", "abc", strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if _, err := ParseSHA256(bad); err == nil {
			t.Errorf("ParseSHA256(%q): want error", bad)
		}
	}
}

func TestVerifySHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	if err := VerifySHA256(p, hex.EncodeToString(sum[:])); err != nil {
		t.Errorf("VerifySHA256 match: %v", err)
	}
	if err := VerifySHA256(p, strings.Repeat("0", 64)); err == nil {
		t.Error("VerifySHA256 mismatch: want error")
	}
}

func TestInstallDir(t *testing.T) {
	if got, err := InstallDir(`D:\gradles`); err != nil || got != `D:\gradles` {
		t.Errorf("InstallDir(cfg) = %q, %v", got, err)
	}
	got, err := InstallDir("")
	if err != nil {
		t.Fatalf("InstallDir(default): %v", err)
	}
	if !strings.Contains(filepath.ToSlash(got), ".barista/toolchains/gradle") {
		t.Errorf("default install dir = %q", got)
	}
}

func fakeDistribution(t *testing.T, distVersion, jarVersion string) (zipData []byte, sum string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{
		"gradle-" + distVersion + "/bin/gradle",
		"gradle-" + distVersion + "/bin/gradle.bat",
		"gradle-" + distVersion + "/lib/gradle-core-" + jarVersion + ".jar",
		"gradle-" + distVersion + "/lib/gradle-core-api-" + jarVersion + ".jar",
		"gradle-" + distVersion + "/lib/gradle-launcher-" + jarVersion + ".jar",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(s[:])
}

func gradleDistServer(t *testing.T, version string, zipData []byte, inlineSum, fileSum string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/versions/all":
			_, _ = fmt.Fprintf(w, `[{"version":%q,"snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"","milestoneFor":"","downloadUrl":%q,"checksumUrl":%q,"checksum":%q,"current":true,"final":true}]`,
				version, srv.URL+"/gradle-"+version+"-bin.zip", srv.URL+"/gradle-"+version+"-bin.zip.sha256", inlineSum)
		case "/gradle-" + version + "-bin.zip":
			_, _ = w.Write(zipData)
		case "/gradle-" + version + "-bin.zip.sha256":
			_, _ = fmt.Fprintf(w, "%s  gradle-%s-bin.zip\n", fileSum, version)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func installFixture(t *testing.T, inlineSum, fileSum string) (*Registry, string) {
	t.Helper()
	zipData, sum := fakeDistribution(t, "8.10.2", "8.10.2")
	if inlineSum == "real" {
		inlineSum = sum
	}
	if fileSum == "real" {
		fileSum = sum
	}
	srv := gradleDistServer(t, "8.10.2", zipData, inlineSum, fileSum)
	useVersionsBase(t, srv.URL)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "gradle.json")
	return reg, regPath
}

func TestInstall(t *testing.T) {
	reg, regPath := installFixture(t, "real", "")
	res, e := Install(context.Background(), reg, regPath, "8.10.2", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.RequestedVersion != "" {
		t.Errorf("RequestedVersion = %q, want empty", res.RequestedVersion)
	}
	if res.Entry.Name != "gradle-8.10.2" || res.Entry.Version != "8.10.2" || !res.Entry.Managed {
		t.Errorf("entry = %+v", res.Entry)
	}
	for _, rel := range []string{"bin/gradle", "bin/gradle.bat", "lib/gradle-core-8.10.2.jar", "lib/gradle-core-api-8.10.2.jar", "lib/gradle-launcher-8.10.2.jar"} {
		if _, err := os.Stat(filepath.Join(res.Entry.Path, filepath.FromSlash(rel))); err != nil {
			t.Errorf("installed file %s: %v", rel, err)
		}
	}
	loaded, e := Load(regPath)
	if e != nil || len(loaded.Installations) != 1 || loaded.Installations[0].Name != "gradle-8.10.2" {
		t.Errorf("registry after install: %+v, %v", loaded, e)
	}
}

func TestInstallFuzzySubstitution(t *testing.T) {
	reg, regPath := installFixture(t, "real", "")
	res, e := Install(context.Background(), reg, regPath, "8", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.RequestedVersion != "8" || res.Entry.Version != "8.10.2" || res.Entry.Name != "gradle-8.10.2" {
		t.Errorf("result = %+v", res)
	}
}

func TestInstallExplicitName(t *testing.T) {
	reg, regPath := installFixture(t, "real", "")
	res, e := Install(context.Background(), reg, regPath, "8.10.2", "my-gradle", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.Entry.Name != "my-gradle" || filepath.Base(res.Entry.Path) != "my-gradle" {
		t.Errorf("entry = %+v", res.Entry)
	}
}

func TestInstallNameConflict(t *testing.T) {
	useVersionsBase(t, "http://127.0.0.1:1")
	reg := &Registry{InstallDir: t.TempDir()}
	reg.Installations = append(reg.Installations, Entry{Name: "my-gradle", Version: "8.9", Path: "/x"})
	regPath := filepath.Join(t.TempDir(), "gradle.json")
	if _, e := Install(context.Background(), reg, regPath, "8.10.2", "my-gradle", nil); e == nil || e.Code != output.CodeGradleExists {
		t.Errorf("want GRADLE_EXISTS before any network, got %v", e)
	}
}

func TestInstallAutoNameConflictSuffix(t *testing.T) {
	reg, regPath := installFixture(t, "real", "")
	reg.Installations = append(reg.Installations, Entry{Name: "gradle-8.10.2", Version: "8.10.2", Path: "/x"})
	res, e := Install(context.Background(), reg, regPath, "8.10.2", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.Entry.Name != "gradle-8.10.2-1" {
		t.Errorf("name = %q, want gradle-8.10.2-1", res.Entry.Name)
	}
}

func TestInstallNoMatch(t *testing.T) {
	reg, regPath := installFixture(t, "real", "")
	if _, e := Install(context.Background(), reg, regPath, "7", "", nil); e == nil || e.Code != output.CodeGradleNotFound {
		t.Errorf("want GRADLE_NOT_FOUND, got %v", e)
	}
}

func TestInstallPrerelease(t *testing.T) {
	zipData, sum := fakeDistribution(t, "9.0.0-rc-1", "9.0.0")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/versions/all":
			_, _ = fmt.Fprintf(w, `[{"version":"9.0.0-rc-1","snapshot":false,"nightly":false,"releaseNightly":false,"broken":false,"rcFor":"9.0.0","milestoneFor":"","downloadUrl":%q,"checksumUrl":%q,"checksum":%q,"current":false,"final":false}]`,
				srv.URL+"/gradle-9.0.0-rc-1-bin.zip", srv.URL+"/gradle-9.0.0-rc-1-bin.zip.sha256", sum)
		case "/gradle-9.0.0-rc-1-bin.zip":
			_, _ = w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	useVersionsBase(t, srv.URL)
	reg := &Registry{InstallDir: t.TempDir()}
	regPath := filepath.Join(t.TempDir(), "gradle.json")
	res, e := Install(context.Background(), reg, regPath, "9.0.0-rc-1", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.Entry.Name != "gradle-9.0.0-rc-1" || res.Entry.Version != "9.0.0-rc-1" || !res.Entry.Managed {
		t.Errorf("entry = %+v, want the full rc version registered", res.Entry)
	}
	if _, err := os.Stat(filepath.Join(res.Entry.Path, "lib", "gradle-core-9.0.0.jar")); err != nil {
		t.Errorf("base-version jar: %v", err)
	}
	loaded, e := Load(regPath)
	if e != nil || len(loaded.Installations) != 1 || loaded.Installations[0].Version != "9.0.0-rc-1" {
		t.Errorf("registry after install: %+v, %v", loaded, e)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	reg, regPath := installFixture(t, strings.Repeat("0", 64), "")
	if _, e := Install(context.Background(), reg, regPath, "8.10.2", "", nil); e == nil || e.Code != output.CodeGradleChecksumMismatch {
		t.Errorf("want GRADLE_CHECKSUM_MISMATCH, got %v", e)
	}
	entries, err := os.ReadDir(reg.InstallDir)
	if err == nil && len(entries) != 0 {
		t.Errorf("install dir not clean after failure: %v", entries)
	}
}

func TestInstallChecksumURLFallback(t *testing.T) {
	reg, regPath := installFixture(t, "", "real")
	res, e := Install(context.Background(), reg, regPath, "8.10.2", "", nil)
	if e != nil {
		t.Fatalf("Install: %v", e)
	}
	if res.Entry.Version != "8.10.2" {
		t.Errorf("entry = %+v", res.Entry)
	}
}
