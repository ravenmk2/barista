package nodecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"barista/internal/node"
	"barista/internal/output"
	"barista/internal/workspace"
)

func testNodeReg() *node.Registry {
	return &node.Registry{
		Installations: []node.Entry{
			{Name: "node-20.19.0", Version: "20.19.0", Path: "/n/20"},
			{Name: "node-22.14.0", Version: "22.14.0", Path: "/n/22"},
		},
		Default: "node-22.14.0",
	}
}

func wsInput(t *testing.T) planInput {
	t.Helper()
	root := t.TempDir()
	return planInput{
		cwd:        filepath.Join(root, "repos", "app", "src"),
		goos:       "linux",
		tool:       "node",
		ambientBin: "/usr/bin/node",
		wsRoot:     root,
		repos:      []workspace.Repo{{Name: "app", Path: "repos/app"}},
		nodeReg:    testNodeReg(),
	}
}

func TestPlanNodePriority(t *testing.T) {
	withAll := func(in planInput) planInput {
		in.nodeFlag = "20"
		in.fileVersion = "20.19.0"
		in.filePath = filepath.Join(in.wsRoot, "repos", "app", ".node-version")
		in.repos[0].Properties = map[string]string{"node": "node-22.14.0"}
		in.props = workspace.Properties{"node": "20"}
		in.nodeReg.Default = "node-20.19.0"
		return in
	}

	t.Run("flag beats all", func(t *testing.T) {
		in := withAll(wsInput(t))
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.nodeSrc != "flag" || p.nodeName != "node-20.19.0" {
			t.Errorf("want flag→node-20.19.0, got %s→%s", p.nodeSrc, p.nodeName)
		}
	})

	t.Run("file beats repo and below", func(t *testing.T) {
		in := withAll(wsInput(t))
		in.nodeFlag = ""
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.nodeSrc != "version-file" || p.nodeName != "node-20.19.0" || p.nodeFile == "" {
			t.Errorf("want version-file→node-20.19.0, got %s→%s file=%q", p.nodeSrc, p.nodeName, p.nodeFile)
		}
	})

	t.Run("repo beats workspace", func(t *testing.T) {
		in := withAll(wsInput(t))
		in.nodeFlag = ""
		in.fileVersion = ""
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.nodeSrc != "repo" || p.nodeName != "node-22.14.0" || p.nodeHome != "/n/22" {
			t.Errorf("want repo→node-22.14.0, got %s→%s home=%s", p.nodeSrc, p.nodeName, p.nodeHome)
		}
	})

	t.Run("workspace when cwd matches no repo", func(t *testing.T) {
		in := withAll(wsInput(t))
		in.nodeFlag = ""
		in.fileVersion = ""
		in.cwd = in.wsRoot
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.nodeSrc != "workspace" || p.nodeName != "node-20.19.0" {
			t.Errorf("want workspace→node-20.19.0, got %s→%s", p.nodeSrc, p.nodeName)
		}
	})

	t.Run("user default when nothing higher", func(t *testing.T) {
		in := wsInput(t)
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.nodeSrc != "user" || p.nodeName != "node-22.14.0" {
			t.Errorf("want user→node-22.14.0, got %s→%s", p.nodeSrc, p.nodeName)
		}
	})

	t.Run("ambient when nothing declared", func(t *testing.T) {
		in := wsInput(t)
		in.nodeReg.Default = ""
		p, e := planExec(in)
		if e != nil {
			t.Fatal(e)
		}
		if p.nodeSrc != "ambient" || p.bin != "/usr/bin/node" || p.nodeHome != "" {
			t.Errorf("want ambient /usr/bin/node, got src=%s bin=%s home=%s", p.nodeSrc, p.bin, p.nodeHome)
		}
	})
}

func TestPlanUnregisteredSpecFailsLoudly(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		in := wsInput(t)
		in.nodeFlag = "24"
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeNodeNotFound {
			t.Fatalf("want NODE_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, "(from flag)") {
			t.Errorf("message must name the source, got %q", e.Message)
		}
	})

	t.Run("workspace", func(t *testing.T) {
		in := wsInput(t)
		in.props = workspace.Properties{"node": "24"}
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeNodeNotFound {
			t.Fatalf("want NODE_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, "(from workspace)") {
			t.Errorf("message must name the source, got %q", e.Message)
		}
	})

	t.Run("version file names the file", func(t *testing.T) {
		in := wsInput(t)
		in.fileVersion = "24.0.0"
		in.filePath = filepath.Join(in.wsRoot, "repos", "app", ".nvmrc")
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeNodeNotFound {
			t.Fatalf("want NODE_NOT_FOUND, got %+v", e)
		}
		if !strings.Contains(e.Message, ".nvmrc") {
			t.Errorf("message must name .nvmrc, got %q", e.Message)
		}
	})

	t.Run("dangling user default", func(t *testing.T) {
		in := wsInput(t)
		in.nodeReg.Default = "ghost"
		_, e := planExec(in)
		if e == nil || e.Code != output.CodeNodeNotFound {
			t.Fatalf("want NODE_NOT_FOUND, got %+v", e)
		}
	})
}

func TestPlanNoNodeAnywhere(t *testing.T) {
	in := wsInput(t)
	in.nodeReg.Default = ""
	in.ambientBin = ""
	_, e := planExec(in)
	if e == nil || e.Code != output.CodeNodeNotFound {
		t.Errorf("want NODE_NOT_FOUND, got %+v", e)
	}
}

func TestPlanToolBinaryPerGOOS(t *testing.T) {
	base := wsInput(t)
	base.nodeFlag = "22"

	in := base
	in.goos = "linux"
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if want := filepath.Join("/n/22", "bin", "node"); p.bin != want || p.viaCmd {
		t.Errorf("linux node: got %q viaCmd=%v, want %q direct", p.bin, p.viaCmd, want)
	}
	if want := filepath.Join("/n/22", "bin"); p.pathEntry != want {
		t.Errorf("linux pathEntry: got %q, want %q", p.pathEntry, want)
	}

	in = base
	in.goos = "windows"
	p, e = planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if want := filepath.Join("/n/22", "node.exe"); p.bin != want || p.viaCmd {
		t.Errorf("windows node: got %q viaCmd=%v, want %q direct", p.bin, p.viaCmd, want)
	}
	if p.pathEntry != "/n/22" {
		t.Errorf("windows pathEntry: got %q, want the distribution root", p.pathEntry)
	}

	in = base
	in.goos = "windows"
	in.tool = "npm"
	p, e = planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if want := filepath.Join("/n/22", "npm.cmd"); p.bin != want || !p.viaCmd {
		t.Errorf("windows npm: got %q viaCmd=%v, want %q via cmd", p.bin, p.viaCmd, want)
	}

	in = base
	in.goos = "linux"
	in.tool = "npx"
	p, e = planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if want := filepath.Join("/n/22", "bin", "npx"); p.bin != want || p.viaCmd {
		t.Errorf("linux npx: got %q viaCmd=%v, want %q direct", p.bin, p.viaCmd, want)
	}
}

func TestPlanAmbientCmdShim(t *testing.T) {
	base := wsInput(t)
	base.nodeReg.Default = ""
	base.goos = "windows"
	base.tool = "npm"
	base.ambientBin = `C:\Program Files\nodejs\npm.cmd`
	p, e := planExec(base)
	if e != nil {
		t.Fatal(e)
	}
	if p.nodeSrc != "ambient" || !p.viaCmd {
		t.Errorf("ambient .cmd on windows must go through cmd, got src=%s viaCmd=%v", p.nodeSrc, p.viaCmd)
	}

	base.tool = "node"
	base.ambientBin = `C:\Program Files\nodejs\node.exe`
	p, e = planExec(base)
	if e != nil {
		t.Fatal(e)
	}
	if p.viaCmd {
		t.Errorf("ambient node.exe must not be wrapped in cmd, got viaCmd=%v", p.viaCmd)
	}
}

func TestPlanPassthrough(t *testing.T) {
	in := wsInput(t)
	in.passthrough = []string{"--version"}
	p, e := planExec(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.passthrough) != 1 || p.passthrough[0] != "--version" {
		t.Errorf("passthrough mangled: %v", p.passthrough)
	}
}

func TestDetectFilesSwitch(t *testing.T) {
	if !detectFilesEnabled(nil) {
		t.Error("nil props must default to enabled")
	}
	if detectFilesEnabled(workspace.Properties{"detect.files": false}) {
		t.Error("explicit false must disable")
	}
}

func TestDetectNodeVersionFile(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repos", "app")
	deep := filepath.Join(repo, "src")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nvmrc := filepath.Join(repo, ".nvmrc")
	if err := os.WriteFile(nvmrc, []byte("# lts\n\nv20.19.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	v, got, ok := detectNodeVersionFile(deep, root)
	if !ok || v != "20.19.0" || got != nvmrc {
		t.Errorf("nvmrc: got (%q, %q, %v), want (20.19.0, %q, true)", v, got, ok, nvmrc)
	}

	nodeVersion := filepath.Join(repo, ".node-version")
	if err := os.WriteFile(nodeVersion, []byte("22.14.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, got, ok = detectNodeVersionFile(deep, root)
	if !ok || v != "22.14.0" || got != nodeVersion {
		t.Errorf(".node-version must win over .nvmrc: got (%q, %q, %v)", v, got, ok)
	}

	if _, _, ok := detectNodeVersionFile(root, root); ok {
		t.Error("files above the boundary must not be detected")
	}

	unparseable := t.TempDir()
	if err := os.WriteFile(filepath.Join(unparseable, ".nvmrc"), []byte("lts/*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := detectNodeVersionFile(unparseable, ""); ok {
		t.Error("unparseable content must not be detected")
	}
}

func TestChildEnv(t *testing.T) {
	env := []string{"NODE_HOME=/old", `PATH=C:\Windows`, "HOME=/home/x"}
	got := childEnv(env, `D:\nodes\22`, `D:\nodes\22`)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `NODE_HOME=D:\nodes\22`) || strings.Contains(joined, "NODE_HOME=/old") {
		t.Errorf("NODE_HOME must be replaced: %v", got)
	}
	sep := string(os.PathListSeparator)
	if !strings.Contains(joined, `PATH=D:\nodes\22`+sep+`C:\Windows`) {
		t.Errorf("bin dir must be prepended to PATH: %v", got)
	}
	if !strings.Contains(joined, "HOME=/home/x") {
		t.Errorf("unrelated vars must pass through: %v", got)
	}

	got = childEnv([]string{"HOME=/home/x"}, "/n/22", "/n/22/bin")
	if len(got) != 3 {
		t.Errorf("missing NODE_HOME/PATH must be appended: %v", got)
	}

	got = childEnv(env, "", "")
	if strings.Join(got, "\n") != strings.Join(env, "\n") {
		t.Errorf("ambient must leave the environment untouched: %v", got)
	}
}
