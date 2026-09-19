package javacli

import (
	"path/filepath"
	"testing"

	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

func testJdkReg() *jdk.Registry {
	return &jdk.Registry{
		JDKs: []jdk.Entry{
			{Name: "temurin8", Major: 8, Version: "1.8.0_422", Path: "/j/8"},
			{Name: "temurin17", Major: 17, Version: "17.0.12", Path: "/j/17"},
		},
	}
}

func wsInput(t *testing.T) planInput {
	t.Helper()
	root := t.TempDir()
	return planInput{
		cwd:        filepath.Join(root, "repos", "app", "src"),
		goos:       "linux",
		ambientBin: "/usr/bin/java",
		wsRoot:     root,
		repos:      []workspace.Repo{{Name: "app", Path: "repos/app"}},
		jdkReg:     testJdkReg(),
	}
}

func TestPlanJdkPriority(t *testing.T) {
	withAll := func(in planInput) planInput {
		in.jdkFlag = "8"
		in.repos[0].Properties = map[string]string{"jdk": "17"}
		in.props = workspace.Properties{"jdk": "temurin17"}
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

	t.Run("repo beats workspace", func(t *testing.T) {
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

	t.Run("workspace when cwd matches no repo", func(t *testing.T) {
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
	})

	t.Run("ambient when nothing declared", func(t *testing.T) {
		p, e := planExec(wsInput(t))
		if e != nil {
			t.Fatal(e)
		}
		if p.jdkSrc != "ambient" || p.javaBin != "/usr/bin/java" || p.javaHome != "" {
			t.Errorf("want ambient /usr/bin/java, got src=%s bin=%s home=%s", p.jdkSrc, p.javaBin, p.javaHome)
		}
	})
}

func TestPlanUnregisteredSpecFailsLoudly(t *testing.T) {
	in := wsInput(t)
	in.jdkFlag = "21"
	_, e := planExec(in)
	if e == nil {
		t.Fatal("expected error for unregistered jdk")
	}
	if e.Code != output.CodeJDKNotFound {
		t.Errorf("got code %q, want %q", e.Code, output.CodeJDKNotFound)
	}
}

func TestPlanNoJdkAnywhere(t *testing.T) {
	in := wsInput(t)
	in.ambientBin = ""
	_, e := planExec(in)
	if e == nil {
		t.Fatal("expected error when no jdk configured and no java on PATH")
	}
	if e.Code != output.CodeJDKNotFound {
		t.Errorf("got code %q, want %q", e.Code, output.CodeJDKNotFound)
	}
}

func TestPlanJavaBinPerGOOS(t *testing.T) {
	in := wsInput(t)
	in.jdkFlag = "17"

	in.goos = "linux"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if want := filepath.Join("/j/17", "bin", "java"); p.javaBin != want {
		t.Errorf("linux: got %q, want %q", p.javaBin, want)
	}

	in.goos = "windows"
	p, e = planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if want := filepath.Join("/j/17", "bin", "java.exe"); p.javaBin != want {
		t.Errorf("windows: got %q, want %q", p.javaBin, want)
	}
}

func TestPlanPassthrough(t *testing.T) {
	in := wsInput(t)
	in.passthrough = []string{"-jar", "app.jar"}
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.passthrough) != 2 || p.passthrough[0] != "-jar" || p.passthrough[1] != "app.jar" {
		t.Errorf("passthrough mangled: %v", p.passthrough)
	}
}
