package mvncli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

func fakeMavenHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, dir := range []string{"boot", "bin", filepath.Join("lib", "jansi-native")} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{
		filepath.Join("boot", "plexus-classworlds-2.7.0.jar"),
		filepath.Join("bin", "m2.conf"),
	} {
		if err := os.WriteFile(filepath.Join(home, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func jarInput(t *testing.T) planInput {
	t.Helper()
	mavenReg, jdkReg := testRegs()
	mavenReg.Installations[0].Path = fakeMavenHome(t)
	in := planInput{
		cwd:         t.TempDir(),
		goos:        "linux",
		startupFlag: "jar",
		mavenReg:    mavenReg,
		jdkReg:      jdkReg,
		passthrough: []string{"clean"},
	}
	return in
}

func TestResolveStartup(t *testing.T) {
	repo := &workspace.Repo{Name: "app", Path: "repos/app", Properties: map[string]string{"maven.startup": "jar"}}

	t.Run("default is script", func(t *testing.T) {
		spec, src, e := resolveStartup(planInput{}, nil)
		if e != nil || spec != "script" || src != "default" {
			t.Errorf("want script/default, got %s/%s %v", spec, src, e)
		}
	})

	t.Run("flag beats repo and workspace", func(t *testing.T) {
		in := planInput{startupFlag: "script", props: workspace.Properties{"maven.startup": "jar"}}
		spec, src, e := resolveStartup(in, repo)
		if e != nil || spec != "script" || src != "flag" {
			t.Errorf("want script/flag, got %s/%s %v", spec, src, e)
		}
	})

	t.Run("repo beats workspace", func(t *testing.T) {
		in := planInput{props: workspace.Properties{"maven.startup": "script"}}
		spec, src, e := resolveStartup(in, repo)
		if e != nil || spec != "jar" || src != "repo" {
			t.Errorf("want jar/repo, got %s/%s %v", spec, src, e)
		}
	})

	t.Run("workspace property", func(t *testing.T) {
		in := planInput{props: workspace.Properties{"maven.startup": "jar"}}
		spec, src, e := resolveStartup(in, nil)
		if e != nil || spec != "jar" || src != "workspace" {
			t.Errorf("want jar/workspace, got %s/%s %v", spec, src, e)
		}
	})

	t.Run("invalid value is loud", func(t *testing.T) {
		in := planInput{startupFlag: "warp"}
		_, _, e := resolveStartup(in, nil)
		if e == nil || e.Code != output.CodeConfigError {
			t.Fatalf("want CONFIG_ERROR, got %+v", e)
		}
		if !strings.Contains(e.Message, `"warp"`) || !strings.Contains(e.Message, "from flag") {
			t.Errorf("message must name value and source, got %q", e.Message)
		}
	})
}

func TestJarLaunchHappyPath(t *testing.T) {
	in := jarInput(t)
	in.jdkFlag = "17"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if p.startup != "jar" || p.startupSrc != "flag" {
		t.Errorf("want jar/flag, got %s/%s", p.startup, p.startupSrc)
	}
	ls := p.launch
	if ls.viaCmd {
		t.Error("jar mode must not go through cmd")
	}
	wantJava := filepath.Join("/j/17", "bin", javaExeName("linux"))
	if ls.bin != wantJava {
		t.Errorf("want java bin %q, got %q", wantJava, ls.bin)
	}
	home := in.mavenReg.Installations[0].Path
	joined := strings.Join(ls.args, " ")
	for _, want := range []string{
		"-classpath " + filepath.Join(home, "boot", "plexus-classworlds-2.7.0.jar"),
		"-Dclassworlds.conf=" + filepath.Join(home, "bin", "m2.conf"),
		"-Dmaven.home=" + home,
		"-Dlibrary.jansi.path=" + filepath.Join(home, "lib", "jansi-native"),
		"-Dmaven.multiModuleProjectDirectory=" + in.cwd,
		classworldsLauncher,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q\ngot: %s", want, joined)
		}
	}
	if ls.args[len(ls.args)-1] != "clean" {
		t.Errorf("passthrough must be last, got %v", ls.args)
	}
	launcherIdx := indexOf(ls.args, classworldsLauncher)
	cpIdx := indexOf(ls.args, "-classpath")
	if cpIdx < 0 || launcherIdx < 0 || cpIdx > launcherIdx {
		t.Errorf("classpath must precede launcher, got %v", ls.args)
	}
}

func TestJarLaunchAmbientJdk(t *testing.T) {
	p, e := planExec(jarInput(t))
	if e != nil {
		t.Fatal(e)
	}
	if p.launch.bin != "java" {
		t.Errorf("ambient jdk means PATH lookup, got %q", p.launch.bin)
	}
}

func TestJarLaunchBootJarErrors(t *testing.T) {
	t.Run("missing jar", func(t *testing.T) {
		in := jarInput(t)
		home := in.mavenReg.Installations[0].Path
		if err := os.Remove(filepath.Join(home, "boot", "plexus-classworlds-2.7.0.jar")); err != nil {
			t.Fatal(err)
		}
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeMavenExecFailed {
			t.Fatalf("want MAVEN_EXEC_FAILED, got %+v", e)
		}
		if !strings.Contains(e.Hint, "--startup script") {
			t.Errorf("hint must offer the wrapper fallback, got %q", e.Hint)
		}
	})

	t.Run("two jars", func(t *testing.T) {
		in := jarInput(t)
		home := in.mavenReg.Installations[0].Path
		extra := filepath.Join(home, "boot", "plexus-classworlds-2.8.0.jar")
		if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeMavenExecFailed {
			t.Fatalf("want MAVEN_EXEC_FAILED, got %+v", e)
		}
	})

	t.Run("missing m2.conf", func(t *testing.T) {
		in := jarInput(t)
		home := in.mavenReg.Installations[0].Path
		if err := os.Remove(filepath.Join(home, "bin", "m2.conf")); err != nil {
			t.Fatal(err)
		}
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeMavenExecFailed {
			t.Fatalf("want MAVEN_EXEC_FAILED, got %+v", e)
		}
	})
}

func TestFindBasedir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "module", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".mvn"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("walks up to .mvn", func(t *testing.T) {
		if got := findBasedir(nested, nil); got != root {
			t.Errorf("want %q, got %q", root, got)
		}
	})

	t.Run("falls back to cwd", func(t *testing.T) {
		bare := t.TempDir()
		if got := findBasedir(bare, nil); got != bare {
			t.Errorf("want %q, got %q", bare, got)
		}
	})

	t.Run("-f directory target", func(t *testing.T) {
		if got := findBasedir(nested, []string{"-f", root}); got != root {
			t.Errorf("want %q, got %q", root, got)
		}
	})

	t.Run("-f file target", func(t *testing.T) {
		pom := filepath.Join(root, "pom.xml")
		if err := os.WriteFile(pom, []byte("<project/>"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := findBasedir(nested, []string{"--file", pom}); got != root {
			t.Errorf("want %q, got %q", root, got)
		}
	})

	t.Run("MAVEN_BASEDIR env wins", func(t *testing.T) {
		env := t.TempDir()
		t.Setenv("MAVEN_BASEDIR", env)
		if got := findBasedir(nested, nil); got != env {
			t.Errorf("want %q, got %q", env, got)
		}
	})
}

func TestJvmConfigTokens(t *testing.T) {
	root := t.TempDir()
	mvnDir := filepath.Join(root, ".mvn")
	if err := os.MkdirAll(mvnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "-Xmx2g  -Dfoo=bar\n\n-Dbaz=qux\n"
	if err := os.WriteFile(filepath.Join(mvnDir, "jvm.config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	got := jvmConfigTokens(root)
	want := []string{"-Xmx2g", "-Dfoo=bar", "-Dbaz=qux"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("want %v, got %v", want, got)
	}
	if got := jvmConfigTokens(t.TempDir()); got != nil {
		t.Errorf("no config file must yield nil, got %v", got)
	}
}

func TestSupportsMavenArgs(t *testing.T) {
	cases := map[string]bool{
		"3.9.11": true,
		"3.9.0":  true,
		"3.8.1":  false,
		"3.6.0":  false,
		"4.0.0":  true,
		"bogus":  false,
	}
	for v, want := range cases {
		if got := supportsMavenArgs(v); got != want {
			t.Errorf("supportsMavenArgs(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestJarLaunchEnvInjection(t *testing.T) {
	t.Setenv("MAVEN_OPTS", "-Xmx4g -Dstyle.color=always")
	t.Setenv("MAVEN_ARGS", "--show-version")

	t.Run("maven 3.9 appends MAVEN_OPTS and MAVEN_ARGS", func(t *testing.T) {
		in := jarInput(t)
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		args := p.launch.args
		if indexOf(args, "-Xmx4g") < 0 || indexOf(args, "-Dstyle.color=always") < 0 {
			t.Errorf("MAVEN_OPTS missing from %v", args)
		}
		sv := indexOf(args, "--show-version")
		if sv < 0 || sv > indexOf(args, "clean") {
			t.Errorf("MAVEN_ARGS must sit between launcher and passthrough, got %v", args)
		}
	})

	t.Run("maven 3.8 ignores MAVEN_ARGS", func(t *testing.T) {
		in := jarInput(t)
		in.mavenReg.Installations = append(in.mavenReg.Installations, maven.Entry{Name: "maven-3.8", Version: "3.8.1", Path: in.mavenReg.Installations[0].Path})
		in.mavenReg.Default = "maven-3.8"
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if indexOf(p.launch.args, "--show-version") >= 0 {
			t.Errorf("3.8 must not read MAVEN_ARGS, got %v", p.launch.args)
		}
	})
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}
