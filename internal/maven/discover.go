package maven

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"barista/internal/workspace"
)

type Candidate struct {
	Path   string
	Source string
}

func DiscoverCandidates() []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	add := func(path, source string) {
		if path == "" {
			return
		}
		path = filepath.Clean(workspace.ExpandHome(path))
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Candidate{Path: path, Source: source})
	}
	addHome := func(path, source string) {
		if path != "" && hasMvn(path) {
			add(path, source)
		}
	}

	for _, k := range []string{"MAVEN_HOME", "M2_HOME"} {
		add(os.Getenv(k), "env:"+k)
	}

	if home, err := os.UserHomeDir(); err == nil {
		for _, c := range scanDir(filepath.Join(home, ".sdkman", "candidates", "maven")) {
			addHome(c, "sdkman")
		}
		addHome(filepath.Join(home, "scoop", "apps", "maven", "current"), "scoop")
	}

	for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
		for _, c := range scanDir(filepath.Join(prefix, "Cellar", "maven")) {
			addHome(filepath.Join(c, "libexec"), "brew")
		}
	}

	for _, d := range []string{"/usr/share/maven", "/opt/maven"} {
		addHome(d, "system")
	}

	for _, name := range []string{"mvn", "mvn.cmd"} {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if bin := filepath.Dir(p); strings.EqualFold(filepath.Base(bin), "bin") {
			add(filepath.Dir(bin), "path")
		}
	}
	return out
}

func scanDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}
