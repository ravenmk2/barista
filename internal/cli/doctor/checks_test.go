package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/node"
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

func makeGradleHome(t *testing.T, root, name, version string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"gradle", "gradle.bat"} {
		if err := os.WriteFile(filepath.Join(dir, "bin", script), []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, jar := range []string{"gradle-core-", "gradle-core-api-", "gradle-launcher-"} {
		if err := os.WriteFile(filepath.Join(dir, "lib", jar+version+".jar"), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
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
	e := maven.Entry{Name: "maven-3.9.9", Version: "3.9.9", Path: home}
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

func TestCheckGradleEntry(t *testing.T) {
	root := t.TempDir()
	home := makeGradleHome(t, root, "g-8.10", "8.10.2")
	e := gradle.Entry{Name: "gradle-8.10.2", Version: "8.10.2", Path: home}
	if r := checkGradleEntry(e); r.Status != output.StatusOK {
		t.Errorf("want ok, got %v", r.Error)
	}
	pre := makeGradleHome(t, root, "g-9.0", "9.0.0")
	e = gradle.Entry{Name: "gradle-9.0.0-rc-1", Version: "9.0.0-rc-1", Path: pre}
	if r := checkGradleEntry(e); r.Status != output.StatusOK {
		t.Errorf("prerelease home probes its base version, want ok, got %v", r.Error)
	}
	e = gradle.Entry{Name: "gradle-8.10.2", Version: "8.11", Path: home}
	if r := checkGradleEntry(e); r.Status != output.StatusFailed {
		t.Errorf("version mismatch want failed, got %s", r.Status)
	}
	e = gradle.Entry{Name: "gradle-8.10.2", Version: "8.10.2", Path: filepath.Join(root, "gone")}
	if r := checkGradleEntry(e); r.Status != output.StatusFailed {
		t.Errorf("missing home want failed, got %s", r.Status)
	}
}

func TestCheckGradleDefaultAndJdk(t *testing.T) {
	reg := &gradle.Registry{}
	if r := checkGradleDefault(reg); r.Status != output.StatusSkipped {
		t.Errorf("unset default want skipped, got %s", r.Status)
	}
	reg.Default = "ghost"
	if r := checkGradleDefault(reg); r.Status != output.StatusFailed {
		t.Errorf("dangling default want failed, got %s", r.Status)
	}
	reg.Jdk = "17"
	jdkReg := &jdk.Registry{}
	if r := checkGradleJdk(reg, jdkReg); r.Status != output.StatusFailed {
		t.Errorf("unresolvable gradle jdk want failed, got %s", r.Status)
	}
	jdkReg.JDKs = []jdk.Entry{{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"}}
	if r := checkGradleJdk(reg, jdkReg); r.Status != output.StatusOK {
		t.Errorf("resolvable gradle jdk want ok, got %v", r.Error)
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

func TestCheckMavenLaunch(t *testing.T) {
	for _, v := range []string{"java", "script"} {
		if r := checkMavenLaunch("workspace", "properties.json", v, "workspace properties"); r.Status != output.StatusOK {
			t.Errorf("%s want ok, got %v", v, r.Error)
		}
	}
	if r := checkMavenLaunch("workspace", "app", "fast", "repo properties"); r.Status != output.StatusFailed {
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

func makeNodeHome(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	bin := node.BinaryPath(dir)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckNodeEntry(t *testing.T) {
	root := t.TempDir()
	home := makeNodeHome(t, root, "node-22")
	e := node.Entry{Name: "node-22.14.0", Version: "22.14.0", Path: home}
	if r := checkNodeEntry(e); r.Status != output.StatusOK {
		t.Errorf("want ok, got %v", r.Error)
	}
	e.Path = filepath.Join(root, "gone")
	r := checkNodeEntry(e)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeNodeNotFound {
		t.Errorf("want failed NODE_NOT_FOUND, got %v", r)
	}
	if r.Error.Hint == "" {
		t.Error("failed check must carry a fix hint")
	}
}

func TestCheckNodeDefault(t *testing.T) {
	reg := &node.Registry{}
	if r := checkNodeDefault(reg); r.Status != output.StatusSkipped {
		t.Errorf("unset default want skipped, got %s", r.Status)
	}
	reg.Default = "ghost"
	if r := checkNodeDefault(reg); r.Status != output.StatusFailed {
		t.Errorf("dangling default want failed, got %s", r.Status)
	}
	reg.Installations = []node.Entry{{Name: "ghost", Version: "22.14.0", Path: "/n/22"}}
	if r := checkNodeDefault(reg); r.Status != output.StatusOK {
		t.Errorf("resolvable default want ok, got %v", r.Error)
	}
}

func TestCheckNodeSpec(t *testing.T) {
	nodeReg := &node.Registry{Installations: []node.Entry{{Name: "node-22.14.0", Version: "22.14.0", Path: "/n/22"}}}
	if r := checkNodeSpec("workspace", "properties.json", "22", "workspace properties", nodeReg); r.Status != output.StatusOK {
		t.Errorf("prefix-resolvable spec want ok, got %v", r.Error)
	}
	if r := checkNodeSpec("workspace", "app", "node-22.14.0", "repo properties", nodeReg); r.Status != output.StatusOK {
		t.Errorf("name spec want ok, got %v", r.Error)
	}
	r := checkNodeSpec("workspace", "properties.json", "24", "workspace properties", nodeReg)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeNodeNotFound {
		t.Errorf("want failed NODE_NOT_FOUND, got %v", r)
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
	if r := checkWorkspaceFile(root, "maven", "settings.xml", "settingsFile", "-s"); r.Status != output.StatusSkipped {
		t.Errorf("absent settings.xml want skipped, got %s", r.Status)
	}
	dir := filepath.Join(root, ".barista", "maven")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.xml"), []byte("<settings/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkWorkspaceFile(root, "maven", "settings.xml", "settingsFile", "-s")
	if r.Status != output.StatusOK || r.Detail["injected"] != "-s" {
		t.Errorf("present settings.xml want ok with injection note, got %v", r)
	}
}

func TestCheckGradleInitFile(t *testing.T) {
	root := t.TempDir()
	if r := checkWorkspaceFile(root, "gradle", "init.gradle", "gradleInitFile", "-I"); r.Status != output.StatusSkipped {
		t.Errorf("absent init.gradle want skipped, got %s", r.Status)
	}
	dir := filepath.Join(root, ".barista", "gradle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "init.gradle.kts"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := checkWorkspaceFile(root, "gradle", "init.gradle", "gradleInitFile", "-I"); r.Status != output.StatusSkipped {
		t.Errorf("init.gradle.kts alone must not satisfy init.gradle, got %s", r.Status)
	}
	r := checkWorkspaceFile(root, "gradle", "init.gradle.kts", "gradleInitKtsFile", "-I")
	if r.Status != output.StatusOK || r.Detail["injected"] != "-I" {
		t.Errorf("present init.gradle.kts want ok with injection note, got %v", r)
	}
}

func TestCheckJavaVersionFile(t *testing.T) {
	root := t.TempDir()
	ws := &workspace.Workspace{Root: root, Repos: &workspace.ReposFile{}}
	repo := workspace.Repo{Name: "app", Path: "repos/app"}
	jdkReg := &jdk.Registry{JDKs: []jdk.Entry{{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"}}}
	dir := filepath.Join(root, "repos", "app")

	if r := checkJavaVersionFile(ws, repo, jdkReg); r.Status != output.StatusSkipped {
		t.Errorf("absent file want skipped, got %s", r.Status)
	}

	write := func(content string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".java-version"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("temurin-17\n")
	if r := checkJavaVersionFile(ws, repo, jdkReg); r.Status != output.StatusOK {
		t.Errorf("resolvable file want ok, got %v", r.Error)
	}

	write("21\n")
	r := checkJavaVersionFile(ws, repo, jdkReg)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeJDKNotFound {
		t.Errorf("unregistered major want failed JDK_NOT_FOUND, got %v", r)
	}

	write("garbage\n")
	r = checkJavaVersionFile(ws, repo, jdkReg)
	if r.Status != output.StatusFailed || r.Error.Hint == "" {
		t.Errorf("unparseable file want failed with hint, got %v", r)
	}
}

func TestCheckMavenWrapperFile(t *testing.T) {
	root := t.TempDir()
	ws := &workspace.Workspace{Root: root, Repos: &workspace.ReposFile{}}
	repo := workspace.Repo{Name: "app", Path: "repos/app"}
	mavenReg := &maven.Registry{Installations: []maven.Entry{{Name: "maven-3.9.11", Version: "3.9.11", Path: "/m/3.9"}}}
	dir := filepath.Join(root, "repos", "app", ".mvn", "wrapper")

	if r := checkMavenWrapperFile(ws, repo, mavenReg); r.Status != output.StatusSkipped {
		t.Errorf("absent file want skipped, got %s", r.Status)
	}

	write := func(content string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "maven-wrapper.properties"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("distributionUrl=https://example.com/apache-maven-3.9.9-bin.zip\n")
	if r := checkMavenWrapperFile(ws, repo, mavenReg); r.Status != output.StatusOK {
		t.Errorf("prefix-resolvable version want ok, got %v", r.Error)
	}

	write("distributionUrl=https://example.com/apache-maven-4.0.0-bin.zip\n")
	r := checkMavenWrapperFile(ws, repo, mavenReg)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeMavenNotFound {
		t.Errorf("unregistered version want failed MAVEN_NOT_FOUND, got %v", r)
	}
	if r.Error.Hint != "run: barista maven install 4.0.0" {
		t.Errorf("hint must offer install, got %q", r.Error.Hint)
	}

	write("distributionUrl=https://example.com/maven.zip\n")
	r = checkMavenWrapperFile(ws, repo, mavenReg)
	if r.Status != output.StatusFailed || r.Error.Hint == "" {
		t.Errorf("unparseable file want failed with hint, got %v", r)
	}
}

func TestCheckGradleWrapperFile(t *testing.T) {
	root := t.TempDir()
	ws := &workspace.Workspace{Root: root, Repos: &workspace.ReposFile{}}
	repo := workspace.Repo{Name: "app", Path: "repos/app"}
	gradleReg := &gradle.Registry{Installations: []gradle.Entry{{Name: "gradle-8.10.2", Version: "8.10.2", Path: "/g/8.10"}}}
	dir := filepath.Join(root, "repos", "app", "gradle", "wrapper")

	if r := checkGradleWrapperFile(ws, repo, gradleReg); r.Status != output.StatusSkipped {
		t.Errorf("absent file want skipped, got %s", r.Status)
	}

	write := func(content string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "gradle-wrapper.properties"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("distributionUrl=https://services.gradle.org/distributions/gradle-8.10.1-bin.zip\n")
	if r := checkGradleWrapperFile(ws, repo, gradleReg); r.Status != output.StatusOK {
		t.Errorf("prefix-resolvable version want ok, got %v", r.Error)
	}

	write("distributionUrl=https://services.gradle.org/distributions/gradle-9.0.0-bin.zip\n")
	r := checkGradleWrapperFile(ws, repo, gradleReg)
	if r.Status != output.StatusFailed || r.Error.Code != output.CodeGradleNotFound {
		t.Errorf("unregistered version want failed GRADLE_NOT_FOUND, got %v", r)
	}
	if r.Error.Hint != "run: barista gradle install 9.0.0" {
		t.Errorf("hint must offer install, got %q", r.Error.Hint)
	}

	write("distributionUrl=https://example.com/gradle.zip\n")
	r = checkGradleWrapperFile(ws, repo, gradleReg)
	if r.Status != output.StatusFailed || r.Error.Hint == "" {
		t.Errorf("unparseable file want failed with hint, got %v", r)
	}
}
