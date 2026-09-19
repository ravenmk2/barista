package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

func makeJdkHome(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	bin := "java"
	if runtime.GOOS == "windows" {
		bin = "java.exe"
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", bin), []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func makeMavenHome(t *testing.T, root, name, version string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "mvn"), []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib", "maven-core-"+version+".jar"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckJdkEntry(t *testing.T) {
	root := t.TempDir()
	home := makeJdkHome(t, root, "jdk-17")
	e := jdk.Entry{Name: "temurin17", Major: 17, Version: "17.0.12", Path: home}
	if r := checkJdkEntry(e); r.Status != output.StatusOK {
		t.Errorf("want ok, got %v", r.Error)
	}
	e.Path = filepath.Join(root, "gone")
	r := checkJdkEntry(e)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeJDKNotFound {
		t.Errorf("want failed JDK_NOT_FOUND, got %v", r)
	}
	if r.Error.Hint == "" {
		t.Error("failed check must carry a fix hint")
	}
}

func TestCheckJdkDefaults(t *testing.T) {
	reg := &jdk.Registry{}
	if r := checkJdkDefaults(reg); r.Status != output.StatusSkipped {
		t.Errorf("empty defaults want skipped, got %s", r.Status)
	}
	reg.Defaults = map[string]string{"17": "temurin17"}
	if r := checkJdkDefaults(reg); r.Status != output.StatusFailed {
		t.Errorf("dangling default want failed, got %s", r.Status)
	}
	reg.JDKs = []jdk.Entry{{Name: "temurin17"}}
	if r := checkJdkDefaults(reg); r.Status != output.StatusOK {
		t.Errorf("resolvable default want ok, got %v", r.Error)
	}
}

func TestCheckMavenEntry(t *testing.T) {
	root := t.TempDir()
	home := makeMavenHome(t, root, "m-3.9", "3.9.9")
	e := maven.Entry{Name: "maven-3.9", Version: "3.9.9", Path: home}
	if r := checkMavenEntry(e); r.Status != output.StatusOK {
		t.Errorf("want ok, got %v", r.Error)
	}
	e.Version = "3.9.11"
	if r := checkMavenEntry(e); r.Status != output.StatusFailed {
		t.Errorf("version mismatch want failed, got %s", r.Status)
	}
	e.Path = filepath.Join(root, "gone")
	if r := checkMavenEntry(e); r.Status != output.StatusFailed {
		t.Errorf("missing home want failed, got %s", r.Status)
	}
}

func TestCheckMavenDefaultAndJdk(t *testing.T) {
	reg := &maven.Registry{}
	if r := checkMavenDefault(reg); r.Status != output.StatusSkipped {
		t.Errorf("unset default want skipped, got %s", r.Status)
	}
	reg.Default = "ghost"
	if r := checkMavenDefault(reg); r.Status != output.StatusFailed {
		t.Errorf("dangling default want failed, got %s", r.Status)
	}
	reg.Jdk = "17"
	jdkReg := &jdk.Registry{}
	if r := checkMavenJdk(reg, jdkReg); r.Status != output.StatusFailed {
		t.Errorf("unresolvable maven jdk want failed, got %s", r.Status)
	}
	jdkReg.JDKs = []jdk.Entry{{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"}}
	if r := checkMavenJdk(reg, jdkReg); r.Status != output.StatusOK {
		t.Errorf("resolvable maven jdk want ok, got %v", r.Error)
	}
}

func TestCheckJdkSpec(t *testing.T) {
	jdkReg := &jdk.Registry{JDKs: []jdk.Entry{{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"}}}
	if r := checkJdkSpec("workspace", "properties.json", "17", "workspace properties", jdkReg); r.Status != output.StatusOK {
		t.Errorf("want ok, got %v", r.Error)
	}
	r := checkJdkSpec("workspace", "properties.json", "21", "workspace properties", jdkReg)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeJDKNotFound {
		t.Errorf("want failed JDK_NOT_FOUND, got %v", r)
	}
}

func TestCheckMavenStartup(t *testing.T) {
	for _, v := range []string{"jar", "script"} {
		if r := checkMavenStartup("workspace", "properties.json", v, "workspace properties"); r.Status != output.StatusOK {
			t.Errorf("%s want ok, got %v", v, r.Error)
		}
	}
	if r := checkMavenStartup("workspace", "app", "fast", "repo properties"); r.Status != output.StatusFailed {
		t.Errorf("invalid value want failed, got %s", r.Status)
	}
}

func TestCheckRepoCheckout(t *testing.T) {
	root := t.TempDir()
	ws := &workspace.Workspace{Root: root, Repos: &workspace.ReposFile{}}
	repo := workspace.Repo{Name: "app", Path: "repos/app", ResolvedURL: "https://x/app.git"}

	r := checkRepoCheckout(context.Background(), ws, repo, false)
	if r.Status != output.StatusSkipped {
		t.Errorf("missing checkout want skipped, got %s", r.Status)
	}

	dir := filepath.Join(root, "repos", "app")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r := checkRepoCheckout(context.Background(), ws, repo, false); r.Status != output.StatusOK {
		t.Errorf("shallow with .git want ok, got %v", r.Error)
	}
}

func TestCheckRepoCheckoutDeep(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "repos", "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	git("init", "-q")
	git("remote", "add", "origin", "https://x/app.git")
	ws := &workspace.Workspace{Root: root, Repos: &workspace.ReposFile{}}

	repo := workspace.Repo{Name: "app", Path: "repos/app", ResolvedURL: "https://x/app.git"}
	if r := checkRepoCheckout(context.Background(), ws, repo, true); r.Status != output.StatusOK {
		t.Errorf("matching origin want ok, got %v", r.Error)
	}

	repo.ResolvedURL = "https://x/other.git"
	r := checkRepoCheckout(context.Background(), ws, repo, true)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeRepoRemoteMismatch {
		t.Errorf("mismatched origin want failed REPO_REMOTE_MISMATCH, got %v", r)
	}
}

func TestCheckInstallDir(t *testing.T) {
	if r := checkInstallDir("jdk.json", "jdkInstallDir", "", "reset"); r.Status != output.StatusSkipped {
		t.Errorf("unset installDir want skipped, got %s", r.Status)
	}
	if r := checkInstallDir("jdk.json", "jdkInstallDir", t.TempDir(), "reset"); r.Status != output.StatusOK {
		t.Errorf("existing installDir want ok, got %v", r.Error)
	}
	if r := checkInstallDir("jdk.json", "jdkInstallDir", filepath.Join(t.TempDir(), "gone"), "reset"); r.Status != output.StatusFailed {
		t.Errorf("missing installDir want failed, got %s", r.Status)
	}
}

func TestCheckSettingsFile(t *testing.T) {
	root := t.TempDir()
	if r := checkSettingsFile(root, "settings.xml", "settingsFile", "-s"); r.Status != output.StatusSkipped {
		t.Errorf("absent settings.xml want skipped, got %s", r.Status)
	}
	dir := filepath.Join(root, ".barista", "maven")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.xml"), []byte("<settings/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkSettingsFile(root, "settings.xml", "settingsFile", "-s")
	if r.Status != output.StatusOK || r.Detail["injected"] != "-s" {
		t.Errorf("present settings.xml want ok with injection note, got %v", r)
	}
}
