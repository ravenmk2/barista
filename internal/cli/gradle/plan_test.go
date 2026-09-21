package gradlecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

func testRegs() (*gradle.Registry, *jdk.Registry) {
	gradleReg := &gradle.Registry{
		Installations: []gradle.Entry{{Name: "gradle-8.10", Version: "8.10.2", Path: "/g/8.10"}},
		Default:       "gradle-8.10",
	}
	jdkReg := &jdk.Registry{
		JDKs: []jdk.Entry{
			{Name: "temurin8", Major: 8, Version: "1.8.0_422", Path: "/j/8"},
			{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"},
		},
	}
	return gradleReg, jdkReg
}

func wsInput(t *testing.T) planInput {
	t.Helper()
	root := t.TempDir()
	gradleReg, jdkReg := testRegs()
	return planInput{
		cwd:       filepath.Join(root, "repos", "app", "src"),
		goos:      "linux",
		wsRoot:    root,
		repos:     []workspace.Repo{{Name: "app", Path: "repos/app"}},
		gradleReg: gradleReg,
		jdkReg:    jdkReg,
	}
}

func TestPlanJdkPriority(t *testing.T) {
	withAll := func(in planInput) planInput {
		in.jdkFlag = "8"
		in.repos[0].Properties = map[string]string{"jdk": "17"}
		in.props = workspace.Properties{"jdk": "temurin17"}
		in.gradleReg.Jdk = "temurin8"
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

	t.Run("user gradle.json jdk as last config level", func(t *testing.T) {
		in := wsInput(t)
		in.gradleReg.Jdk = "temurin8"
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
	in.repos[0].Properties = map[string]string{"jdk": "21"}
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

func TestPlanGradleDefault(t *testing.T) {
	t.Run("workspace property wins", func(t *testing.T) {
		in := wsInput(t)
		in.gradleReg.Installations = append(in.gradleReg.Installations, gradle.Entry{Name: "gradle-9", Version: "9.0.0", Path: "/g/9"})
		in.props = workspace.Properties{"gradle.default": "gradle-9"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleName != "gradle-9" || p.gradleSrc != "workspace" {
			t.Errorf("want gradle-9 [workspace], got %s [%s]", p.gradleName, p.gradleSrc)
		}
	})

	t.Run("unregistered workspace default is a loud error", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"gradle.default": "ghost"}
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeGradleNotFound {
			t.Fatalf("want GRADLE_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, "workspace") {
			t.Errorf("message must name the source, got %q", e.Message)
		}
	})

	t.Run("nothing configured", func(t *testing.T) {
		in := wsInput(t)
		in.gradleReg.Default = ""
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeGradleNotFound {
			t.Fatalf("want GRADLE_NOT_FOUND, got %+v", e)
		}
	})
}

func writeInitScript(t *testing.T, wsRoot, name string) string {
	t.Helper()
	dir := filepath.Join(wsRoot, ".barista", "gradle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("// init"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanInitScriptInjection(t *testing.T) {
	t.Run("convention files are injected as -I", func(t *testing.T) {
		in := wsInput(t)
		groovy := writeInitScript(t, in.wsRoot, "init.gradle")
		kts := writeInitScript(t, in.wsRoot, "init.gradle.kts")
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.initScripts) != 2 || p.initScripts[0] != groovy || p.initScripts[1] != kts {
			t.Errorf("want [init.gradle init.gradle.kts], got %v", p.initScripts)
		}
		if len(p.args) < 4 || p.args[0] != "-I" || p.args[1] != groovy || p.args[2] != "-I" || p.args[3] != kts {
			t.Errorf("args must start with -I injections, got %v", p.args)
		}
	})

	t.Run("absent files mean no injection", func(t *testing.T) {
		p, e := planExec(wsInput(t))
		if e != nil {
			t.Fatal(e)
		}
		if len(p.initScripts) != 0 {
			t.Errorf("want no init scripts, got %v", p.initScripts)
		}
	})

	t.Run("user -I suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		writeInitScript(t, in.wsRoot, "init.gradle")
		in.passthrough = []string{"-I", "custom.gradle", "build"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.initScripts) != 0 {
			t.Errorf("user -I must win, got injection %v", p.initScripts)
		}
	})

	t.Run("user --init-script= suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		writeInitScript(t, in.wsRoot, "init.gradle")
		in.passthrough = []string{"--init-script=custom.gradle", "build"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if len(p.initScripts) != 0 {
			t.Errorf("user --init-script must win, got injection %v", p.initScripts)
		}
	})
}

func TestPlanGradleUserHome(t *testing.T) {
	t.Run("relative path anchors to workspace root", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"gradle.user.home": ".barista/gradle-home"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		want := filepath.Join(in.wsRoot, ".barista", "gradle-home")
		if p.gradleUserHome != want {
			t.Errorf("want %q, got %q", want, p.gradleUserHome)
		}
	})

	t.Run("absolute path kept", func(t *testing.T) {
		in := wsInput(t)
		abs := filepath.Join(t.TempDir(), "gradle-home")
		in.props = workspace.Properties{"gradle.user.home": abs}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleUserHome != abs {
			t.Errorf("want %q, got %q", abs, p.gradleUserHome)
		}
	})

	t.Run("user -g suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"gradle.user.home": ".barista/gradle-home"}
		in.passthrough = []string{"-g", "/elsewhere", "build"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleUserHome != "" {
			t.Errorf("user -g must win, got injection %q", p.gradleUserHome)
		}
	})

	t.Run("user --gradle-user-home suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"gradle.user.home": ".barista/gradle-home"}
		in.passthrough = []string{"--gradle-user-home=/elsewhere", "build"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleUserHome != "" {
			t.Errorf("user flag must win, got injection %q", p.gradleUserHome)
		}
	})

	t.Run("user -Dgradle.user.home suppresses injection", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"gradle.user.home": ".barista/gradle-home"}
		in.passthrough = []string{"-Dgradle.user.home=/elsewhere", "build"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleUserHome != "" {
			t.Errorf("user property must win, got injection %q", p.gradleUserHome)
		}
	})
}

func TestPlanGradleBinPerPlatform(t *testing.T) {
	in := wsInput(t)
	in.goos = "windows"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(p.gradleBin, filepath.FromSlash("bin/gradle.bat")) {
		t.Errorf("windows want bin/gradle.bat, got %q", p.gradleBin)
	}
	if !p.launch.viaCmd {
		t.Error("windows must launch via cmd /c")
	}

	in = wsInput(t)
	in.goos = "linux"
	p, e = planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(p.gradleBin, filepath.FromSlash("bin/gradle")) {
		t.Errorf("unix want bin/gradle, got %q", p.gradleBin)
	}
	if p.launch.viaCmd {
		t.Error("unix must exec bin/gradle directly")
	}
}

func TestPlanArgsOrder(t *testing.T) {
	in := wsInput(t)
	writeInitScript(t, in.wsRoot, "init.gradle")
	in.props = workspace.Properties{"gradle.user.home": ".barista/gradle-home"}
	in.passthrough = []string{"clean", "build", "-x", "test"}
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.args) != 8 {
		t.Fatalf("want 8 args, got %v", p.args)
	}
	if p.args[0] != "-I" || p.args[2] != "--gradle-user-home" {
		t.Errorf("injections must lead, got %v", p.args)
	}
	if p.args[4] != "clean" || p.args[7] != "test" {
		t.Errorf("passthrough must follow verbatim, got %v", p.args)
	}
}

func TestPlanOutsideWorkspace(t *testing.T) {
	in := wsInput(t)
	in.wsRoot = ""
	in.repos = nil
	in.cwd = t.TempDir()
	in.gradleReg.Jdk = "temurin17"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if p.gradleSrc != "user" || p.jdkSrc != "user" {
		t.Errorf("want user/user, got %s/%s", p.gradleSrc, p.jdkSrc)
	}
	if p.repoName != "" || len(p.initScripts) != 0 || p.gradleUserHome != "" {
		t.Errorf("no workspace means no repo/init/gradleUserHome, got %+v", p)
	}
}

func TestPlanNestedRepoWins(t *testing.T) {
	in := wsInput(t)
	in.repos = append(in.repos, workspace.Repo{
		Name:       "nested",
		Path:       "repos/app/modules/nested",
		Properties: map[string]string{"jdk": "8"},
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

func TestPlanJavaVersionLevel(t *testing.T) {
	withFile := func(in planInput, distro string) planInput {
		in.javaVersionMajor = 17
		in.javaVersionDistro = distro
		in.javaVersionFile = filepath.Join(in.wsRoot, "repos", "app", ".java-version")
		return in
	}

	t.Run("beats repo workspace and user", func(t *testing.T) {
		in := withFile(wsInput(t), "")
		in.repos[0].Properties = map[string]string{"jdk": "8"}
		in.props = workspace.Properties{"jdk": "temurin8"}
		in.gradleReg.Jdk = "temurin8"
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "java-version" || p.jdkName != "temurin17" || p.jdkFile == "" {
			t.Errorf("want java-version→temurin17, got %s→%s file=%q", p.jdkSrc, p.jdkName, p.jdkFile)
		}
	})

	t.Run("loses to flag", func(t *testing.T) {
		in := withFile(wsInput(t), "")
		in.jdkFlag = "8"
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "flag" || p.jdkName != "temurin8" {
			t.Errorf("want flag→temurin8, got %s→%s", p.jdkSrc, p.jdkName)
		}
	})

	t.Run("distro hint hits named entry", func(t *testing.T) {
		in := withFile(wsInput(t), "temurin")
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSpec != "temurin17" || p.jdkName != "temurin17" {
			t.Errorf("want spec temurin17, got %s→%s", p.jdkSpec, p.jdkName)
		}
	})

	t.Run("distro hint degrades to major", func(t *testing.T) {
		in := withFile(wsInput(t), "zulu")
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "java-version" || p.jdkSpec != "17" || p.jdkName != "temurin17" {
			t.Errorf("want java-version→17→temurin17, got %s spec=%s name=%s", p.jdkSrc, p.jdkSpec, p.jdkName)
		}
	})

	t.Run("unresolvable fails loudly with file source", func(t *testing.T) {
		in := wsInput(t)
		in.javaVersionMajor = 21
		in.javaVersionFile = filepath.Join(in.wsRoot, ".java-version")
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeJDKNotFound {
			t.Fatalf("want JDK_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, ".java-version") {
			t.Errorf("message must name .java-version, got %q", e.Message)
		}
	})
}

func TestPlanWrapperLevel(t *testing.T) {
	t.Run("wrapper beats workspace and user defaults", func(t *testing.T) {
		in := wsInput(t)
		in.gradleReg.Installations = append(in.gradleReg.Installations, gradle.Entry{Name: "gradle-9", Version: "9.0.0-rc-1", Path: "/g/9"})
		in.props = workspace.Properties{"gradle.default": "gradle-9"}
		in.wrapperVersion = "9.0.0"
		in.wrapperFile = filepath.Join(in.wsRoot, "repos", "app", "gradle", "wrapper", "gradle-wrapper.properties")
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleSrc != "wrapper" || p.gradleName != "gradle-9" || p.gradleFile == "" {
			t.Errorf("want wrapper→gradle-9, got %s→%s file=%q", p.gradleSrc, p.gradleName, p.gradleFile)
		}
	})

	t.Run("unregistered wrapper version fails loudly", func(t *testing.T) {
		in := wsInput(t)
		in.wrapperVersion = "9.0.0"
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeGradleNotFound {
			t.Fatalf("want GRADLE_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, "gradle-wrapper.properties") || !strings.Contains(e.Message, "9.0.0") {
			t.Errorf("message must name source and version, got %q", e.Message)
		}
		if !strings.Contains(e.Hint, "barista gradle install 9.0.0") {
			t.Errorf("hint must offer install, got %q", e.Hint)
		}
	})

	t.Run("no wrapper falls through to workspace default", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"gradle.default": "gradle-8.10"}
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.gradleSrc != "workspace" || p.gradleFile != "" {
			t.Errorf("want workspace source, got %s file=%q", p.gradleSrc, p.gradleFile)
		}
	})
}

func TestDetectFilesSwitch(t *testing.T) {
	if !detectFilesEnabled(nil) {
		t.Error("nil props must default to enabled")
	}
	if !detectFilesEnabled(workspace.Properties{"detect.files": true}) {
		t.Error("explicit true must be enabled")
	}
	if detectFilesEnabled(workspace.Properties{"detect.files": false}) {
		t.Error("explicit false must disable")
	}
}

func TestDetectJavaVersionFile(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "app")
	deep := filepath.Join(repo, "src")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo, ".java-version")
	if err := os.WriteFile(file, []byte("temurin-17.0.13\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	major, distro, got := detectJavaVersionFile(deep, root)
	if major != 17 || distro != "temurin" || got != file {
		t.Errorf("got (%d, %q, %q), want (17, temurin, %q)", major, distro, got, file)
	}

	if major, _, _ := detectJavaVersionFile(root, root); major != 0 {
		t.Error("file above boundary must not be detected")
	}

	if err := os.WriteFile(file, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if major, _, _ := detectJavaVersionFile(deep, root); major != 0 {
		t.Error("unparseable file must be treated as absent")
	}
}
