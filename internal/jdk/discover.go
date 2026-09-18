package jdk

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

	envKeys := []string{}
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if isJavaHomeEnv(k) {
			envKeys = append(envKeys, k)
		}
	}
	sort.Strings(envKeys)
	for i, k := range envKeys {
		if k == "JAVA_HOME" {
			envKeys[0], envKeys[i] = envKeys[i], envKeys[0]
			break
		}
	}
	for _, k := range envKeys {
		add(os.Getenv(k), "env:"+k)
	}

	if home, err := os.UserHomeDir(); err == nil {
		for _, c := range scanDir(filepath.Join(home, ".sdkman", "candidates", "java"), "sdkman") {
			add(c.Path, c.Source)
		}
	}

	var sysDirs []string
	switch runtime.GOOS {
	case "linux":
		sysDirs = []string{"/usr/lib/jvm"}
	case "darwin":
		sysDirs = []string{"/Library/Java/JavaVirtualMachines"}
	case "windows":
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			for _, v := range []string{"Java", "Eclipse Adoptium", "Microsoft", "Amazon Corretto", "Zulu", "BellSoft", "SapMachine"} {
				sysDirs = append(sysDirs, filepath.Join(pf, v))
			}
		}
	}
	for _, d := range sysDirs {
		for _, c := range scanDir(d, "system") {
			add(c.Path, c.Source)
		}
	}

	if javaBin, err := exec.LookPath(exeName("java")); err == nil {
		if resolved, err := filepath.EvalSymlinks(javaBin); err == nil {
			if bin := filepath.Dir(resolved); filepath.Base(bin) == "bin" {
				add(filepath.Dir(bin), "path")
			}
		}
	}
	return out
}

func isJavaHomeEnv(key string) bool {
	return strings.HasPrefix(key, "JAVA_HOME_") ||
		(strings.HasPrefix(key, "JAVA") && strings.HasSuffix(key, "_HOME"))
}

func scanDir(dir, source string) []Candidate {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, Candidate{Path: filepath.Join(dir, e.Name()), Source: source})
		}
	}
	return out
}

func SamePath(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
