package mvncli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

func testRegs() (*maven.Registry, *jdk.Registry) {
	mavenReg := &maven.Registry{
		Installations: []maven.Entry{{Name: "maven-3.9", Version: "3.9.11", Path: "/m/3.9"}},
		Default:       "maven-3.9",
	}
	jdkReg := &jdk.Registry{
		JDKs: []jdk.Entry{
			{Name: "temurin8", Major: 8, Version: "1.8.0_422", Path: "/j/8"},
			{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"},
		},
	}
	return mavenReg, jdkReg
}

func wsInput(t *testing.T) planInput {
	t.Helper()
	root := t.TempDir()
	mavenReg, jdkReg := testRegs()
	return planInput{
		cwd:      filepath.Join(root, "repos", "app", "src"),
		goos:     "linux",
		wsRoot:   root,
		repos:    []workspace.Repo{{Name: "app", Path: "repos/app"}},
		mavenReg: mavenReg,
		jdkReg:   jdkReg,
	}
}

func TestPlanJdkPriority(t *testing.T) {
	withAll := func(in planInput) planInput {
		in.jdkFlag = "8"
		in.repos[0].Properties = map[string]string{"maven.jdk": "17"}
		in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.jdk": "temurin17"}}
		in.mavenReg.Jdk = "temurin8"
		return in
	}

	t.Run("flag beats all", func(t *testing.T) {
		p, e := planExec(withAll(wsInput(t)))
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "flag" || p.jdkName != "temurin8" {
			t.Errorf("want flag→temurin8, got %s→%s", p.jdkSrc, p.jdkName)
		}
	})

	t.Run("repo beats workspace and user", func(t *testing.T) {
		in := withAll(wsInput(t))
		in.jdkFlag = ""
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "repo" || p.jdkName != "temurin17" || p.javaHome != "/j/17" {
			t.Errorf("want repo→temurin17, got %s→%s home=%s", p.jdkSrc, p.jdkName, p.javaHome)
		}
	})

	t.Run("workspace beats user when cwd matches no repo", func(t *testing.T) {
		in := withAll(wsInput(t))
		in.jdkFlag = ""
		in.cwd = in.wsRoot
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "workspace" || p.jdkName != "temurin17" {
			t.Errorf("want workspace→temurin17, got %s→%s", p.jdkSrc, p.jdkName)
		}
		if p.repoName != "" {
			t.Errorf("workspace root must match no repo, got %q", p.repoName)
		}
	})

	t.Run("user maven.json jdk as last config level", func(t *testing.T) {
		in := wsInput(t)
		in.mavenReg.Jdk = "temurin8"
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "user" || p.jdkName != "temurin8" {
			t.Errorf("want user→temurin8, got %s→%s", p.jdkSrc, p.jdkName)
		}
	})

	t.Run("ambient when nothing configured", func(t *testing.T) {
		p, e := planExec(wsInput(t))
		if e != nil {
			t.Fatal(e)
		}
		if p.javaHome != "" || p.jdkSrc != "" {
			t.Errorf("want ambient, got home=%s src=%s", p.javaHome, p.jdkSrc)
		}
	})
}

func TestPlanJdkUnregistered(t *testing.T) {
	in := wsInput(t)
	in.repos[0].Properties = map[string]string{"maven.jdk": "21"}
	_, e := planExec(in)
	if e == nil {
		t.Fatal("want error")
	}
	if e.Code != output.CodeJDKNotFound {
		t.Errorf("want JDK_NOT_FOUND, got %s", e.Code)
	}
	if !strings.Contains(e.Message, `"21"`) || !strings.Contains(e.Message, "from repo") {
		t.Errorf("message must name spec and source, got %q", e.Message)
	}
}

func TestPlanMavenDefault(t *testing.T) {
	t.Run("workspace property wins", func(t *testing.T) {
		in := wsInput(t)
		in.mavenReg.Installations = append(in.mavenReg.Installations, maven.Entry{Name: "maven-4", Version: "4.0.0", Path: "/m/4"})
		in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.default": "maven-4"}}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.mavenName != "maven-4" || p.mavenSrc != "workspace" {
			t.Errorf("want maven-4 [workspace], got %s [%s]", p.mavenName, p.mavenSrc)
		}
	})

	t.Run("unregistered workspace default is a loud error", func(t *testing.T) {
		in := wsInput(t)
		in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.default": "ghost"}}
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeMavenNotFound {
			t.Fatalf("want MAVEN_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, "workspace") {
			t.Errorf("message must name the source, got %q", e.Message)
		}
	})

	t.Run("nothing configured", func(t *testing.T) {
		in := wsInput(t)
		in.mavenReg.Default = ""
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeMavenNotFound {
			t.Fatalf("want MAVEN_NOT_FOUND, got %+v", e)
		}
	})
}

func writeSettings(t *testing.T, wsRoot, name string) string {
	t.Helper()
	dir := filepath.Join(wsRoot, ".barista", "maven")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("<settings/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanSettingsInjection(t *testing.T) {
	t.Run("convention file is injected as -s", func(t *testing.T) {
		in := wsInput(t)
		s := writeSettings(t, in.wsRoot, "settings.xml")
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.settings != s {
			t.Errorf("want %q, got %q", s, p.settings)
		}
		if len(p.args) < 2 || p.args[0] != "-s" || p.args[1] != s {
			t.Errorf("args must start with -s injection, got %v", p.args)
		}
	})

	t.Run("absent file means no injection", func(t *testing.T) {
		p, e := planExec(wsInput(t))
		if e != nil {
			t.Fatal(e)
		}
		if p.settings != "" {
			t.Errorf("want no settings, got %q", p.settings)
		}
	})

	t.Run("user -s suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		writeSettings(t, in.wsRoot, "settings.xml")
		in.passthrough = []string{"-s", "custom.xml", "package"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.settings != "" {
			t.Errorf("user -s must win, got injection %q", p.settings)
		}
	})

	t.Run("user --settings= suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		writeSettings(t, in.wsRoot, "settings.xml")
		in.passthrough = []string{"--settings=custom.xml", "package"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.settings != "" {
			t.Errorf("user --settings must win, got injection %q", p.settings)
		}
	})

	t.Run("settings-security follows settings", func(t *testing.T) {
		in := wsInput(t)
		writeSettings(t, in.wsRoot, "settings.xml")
		sec := writeSettings(t, in.wsRoot, "settings-security.xml")
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.settingsSecurity != sec {
			t.Errorf("want %q, got %q", sec, p.settingsSecurity)
		}
	})
}

func TestPlanRepoLocal(t *testing.T) {
	t.Run("relative path anchors to workspace root", func(t *testing.T) {
		in := wsInput(t)
		in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.repo.local": ".barista/m2"}}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		want := filepath.Join(in.wsRoot, ".barista", "m2")
		if p.repoLocal != want {
			t.Errorf("want %q, got %q", want, p.repoLocal)
		}
	})

	t.Run("absolute path kept", func(t *testing.T) {
		in := wsInput(t)
		abs := filepath.Join(t.TempDir(), "repo")
		in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.repo.local": abs}}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.repoLocal != abs {
			t.Errorf("want %q, got %q", abs, p.repoLocal)
		}
	})

	t.Run("user -Dmaven.repo.local suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.repo.local": ".barista/m2"}}
		in.passthrough = []string{"-Dmaven.repo.local=/elsewhere", "package"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.repoLocal != "" {
			t.Errorf("user property must win, got injection %q", p.repoLocal)
		}
	})
}

func TestPlanMavenBinPerPlatform(t *testing.T) {
	in := wsInput(t)
	in.goos = "windows"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(p.mavenBin, filepath.FromSlash("bin/mvn.cmd")) {
		t.Errorf("windows want bin/mvn.cmd, got %q", p.mavenBin)
	}

	in = wsInput(t)
	in.goos = "linux"
	p, e = planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(p.mavenBin, filepath.FromSlash("bin/mvn")) {
		t.Errorf("unix want bin/mvn, got %q", p.mavenBin)
	}
}

func TestPlanArgsOrder(t *testing.T) {
	in := wsInput(t)
	writeSettings(t, in.wsRoot, "settings.xml")
	in.wsCfg = workspace.ConfigFile{Properties: map[string]any{"maven.repo.local": ".barista/m2"}}
	in.passthrough = []string{"clean", "install", "-DskipTests"}
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.args) != 6 {
		t.Fatalf("want 6 args, got %v", p.args)
	}
	if p.args[0] != "-s" || !strings.HasPrefix(p.args[2], "-Dmaven.repo.local=") {
		t.Errorf("injections must lead, got %v", p.args)
	}
	if p.args[3] != "clean" || p.args[5] != "-DskipTests" {
		t.Errorf("passthrough must follow verbatim, got %v", p.args)
	}
}

func TestPlanOutsideWorkspace(t *testing.T) {
	in := wsInput(t)
	in.wsRoot = ""
	in.repos = nil
	in.wsCfg = workspace.ConfigFile{}
	in.cwd = t.TempDir()
	in.mavenReg.Jdk = "temurin17"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if p.mavenSrc != "user" || p.jdkSrc != "user" {
		t.Errorf("want user/user, got %s/%s", p.mavenSrc, p.jdkSrc)
	}
	if p.repoName != "" || p.settings != "" || p.repoLocal != "" {
		t.Errorf("no workspace means no repo/settings/repoLocal, got %+v", p)
	}
}

func TestPlanNestedRepoWins(t *testing.T) {
	in := wsInput(t)
	in.repos = append(in.repos, workspace.Repo{
		Name:       "nested",
		Path:       "repos/app/modules/nested",
		Properties: map[string]string{"maven.jdk": "8"},
	})
	in.cwd = filepath.Join(in.wsRoot, "repos", "app", "modules", "nested", "src")
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if p.repoName != "nested" || p.jdkName != "temurin8" {
		t.Errorf("want nested→temurin8, got %s→%s", p.repoName, p.jdkName)
	}
}
