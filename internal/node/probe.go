package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"barista/internal/output"
	"barista/internal/toolversion"
)

type Info struct {
	Home    string
	Version string
}

var execNodeVersion = func(bin string) ([]byte, error) {
	return exec.Command(bin, "--version").Output()
}

// BinaryPath returns the node executable inside an installation home: the
// distribution root on Windows, bin/node elsewhere.
func BinaryPath(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "node.exe")
	}
	return filepath.Join(home, "bin", "node")
}

// BinDir returns the PATH entry for an installation home: the distribution
// root on Windows (node.exe/npm.cmd live there), bin/ elsewhere.
func BinDir(home string) string {
	if runtime.GOOS == "windows" {
		return home
	}
	return filepath.Join(home, "bin")
}

func Probe(path string) (Info, *output.ErrInfo) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, &output.ErrInfo{Code: output.CodeNotANode, Message: err.Error()}
	}
	bin := BinaryPath(abs)
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		want := "bin/node"
		if runtime.GOOS == "windows" {
			want = "node.exe"
		}
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNotANode,
			Message: fmt.Sprintf("%s: no %s found (not a node home)", filepath.ToSlash(abs), want),
		}
	}
	out, err := execNodeVersion(bin)
	if err != nil {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNodeProbeFailed,
			Message: fmt.Sprintf("%s: %v", filepath.ToSlash(bin), err),
		}
	}
	version, err := ParseNodeVersion(string(out))
	if err != nil {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNodeProbeFailed,
			Message: fmt.Sprintf("%s: %v", filepath.ToSlash(bin), err),
		}
	}
	return Info{Home: abs, Version: version}, nil
}

// ParseNodeVersion parses the output of `node --version` (v22.14.0) into the
// bare version string.
func ParseNodeVersion(out string) (string, error) {
	s := strings.TrimSpace(out)
	if !strings.HasPrefix(s, "v") {
		return "", fmt.Errorf("unexpected node --version output %q", s)
	}
	v := strings.TrimPrefix(s, "v")
	if _, _, err := toolversion.Parse(v); err != nil {
		return "", fmt.Errorf("unexpected node --version output %q", s)
	}
	return v, nil
}
