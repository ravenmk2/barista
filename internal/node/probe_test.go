package node

import (
	"os"
	"path/filepath"
	"testing"

	"barista/internal/output"
)

func fakeHome(t *testing.T, withBinary bool) string {
	t.Helper()
	home := t.TempDir()
	if withBinary {
		bin := BinaryPath(home)
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func useExecNodeVersion(t *testing.T, out string) {
	t.Helper()
	orig := execNodeVersion
	execNodeVersion = func(string) ([]byte, error) { return []byte(out), nil }
	t.Cleanup(func() { execNodeVersion = orig })
}

func TestProbe(t *testing.T) {
	useExecNodeVersion(t, "v22.14.0\n")
	home := fakeHome(t, true)
	info, e := Probe(home)
	if e != nil {
		t.Fatalf("Probe: %v", e)
	}
	if info.Version != "22.14.0" || info.Home != home {
		t.Errorf("Probe = %+v, want version 22.14.0 home %s", info, home)
	}
}

func TestProbeNotANode(t *testing.T) {
	home := fakeHome(t, false)
	if _, e := Probe(home); e == nil || e.Code != output.CodeNotANode {
		t.Errorf("want NOT_A_NODE, got %v", e)
	}
}

func TestProbeUnparseableOutput(t *testing.T) {
	useExecNodeVersion(t, "not a version\n")
	home := fakeHome(t, true)
	if _, e := Probe(home); e == nil || e.Code != output.CodeNodeProbeFailed {
		t.Errorf("want NODE_PROBE_FAILED, got %v", e)
	}
}

func TestParseNodeVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v22.14.0\n": "22.14.0",
		"v22.14.0":   "22.14.0",
		"v0.12.18":   "0.12.18",
	} {
		got, err := ParseNodeVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseNodeVersion(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "22.14.0", "v", "vX.Y.Z", "hello\nv22.14.0"} {
		if _, err := ParseNodeVersion(bad); err == nil {
			t.Errorf("ParseNodeVersion(%q): want error", bad)
		}
	}
}

func TestToolBinaryPath(t *testing.T) {
	cases := []struct {
		goos, home, tool string
		wantBin          string
		wantViaCmd       bool
	}{
		{"windows", `C:\nodes\22`, "node", filepath.Join(`C:\nodes\22`, "node.exe"), false},
		{"windows", `C:\nodes\22`, "npm", filepath.Join(`C:\nodes\22`, "npm.cmd"), true},
		{"windows", `C:\nodes\22`, "npx", filepath.Join(`C:\nodes\22`, "npx.cmd"), true},
		{"linux", "/opt/node22", "node", filepath.Join("/opt/node22", "bin", "node"), false},
		{"linux", "/opt/node22", "npm", filepath.Join("/opt/node22", "bin", "npm"), false},
		{"darwin", "/opt/node22", "npx", filepath.Join("/opt/node22", "bin", "npx"), false},
	}
	for _, tc := range cases {
		bin, viaCmd := ToolBinaryPath(tc.goos, tc.home, tc.tool)
		if bin != tc.wantBin || viaCmd != tc.wantViaCmd {
			t.Errorf("ToolBinaryPath(%q, %q, %q) = %q, %v; want %q, %v", tc.goos, tc.home, tc.tool, bin, viaCmd, tc.wantBin, tc.wantViaCmd)
		}
	}
}

func TestBinDirFor(t *testing.T) {
	if got := BinDirFor("windows", `C:\nodes\22`); got != `C:\nodes\22` {
		t.Errorf("windows BinDirFor = %q", got)
	}
	if got := BinDirFor("linux", "/opt/node22"); got != filepath.Join("/opt/node22", "bin") {
		t.Errorf("linux BinDirFor = %q", got)
	}
}
