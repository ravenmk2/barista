package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Init creates <root>/.barista with a repos.json skeleton. Returns false
// when repos.json already exists (never overwritten).
func Init(root, baseURL, defaultBranch string) (bool, error) {
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s is not an existing directory", filepath.ToSlash(root))}
	}
	if err := os.MkdirAll(filepath.Join(root, ".barista"), 0o755); err != nil {
		return false, &LoadError{Code: "CONFIG_ERROR", Message: err.Error()}
	}
	p := filepath.Join(root, ".barista", "repos.json")
	if _, err := os.Stat(p); err == nil {
		return false, nil
	}
	rf := ReposFile{BaseURL: baseURL, DefaultBranch: defaultBranch, Repos: []Repo{}}
	out, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return false, &LoadError{Code: "CONFIG_ERROR", Message: err.Error()}
	}
	out = append(out, '\n')
	if err := writeFileAtomic(p, out); err != nil {
		return false, err
	}
	return true, nil
}

// ScanCheckouts finds git work trees up to two levels below root (direct
// children and grandchildren, e.g. repos/<name>) and returns their paths
// relative to root, sorted. .barista and hidden directories are skipped.
func ScanCheckouts(root string) ([]string, error) {
	var out []string
	levels := [][]string{{""}}
	for depth := 0; depth < 2; depth++ {
		var next []string
		for _, rel := range levels[depth] {
			entries, err := os.ReadDir(filepath.Join(root, rel))
			if err != nil {
				if depth == 0 {
					return nil, &LoadError{Code: "CONFIG_ERROR", Message: err.Error()}
				}
				continue
			}
			for _, e := range entries {
				if !e.IsDir() || e.Name() == ".barista" || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				sub := filepath.Join(rel, e.Name())
				if fi, err := os.Stat(filepath.Join(root, sub, ".git")); err == nil && fi.IsDir() {
					out = append(out, filepath.ToSlash(sub))
					continue
				}
				next = append(next, sub)
			}
		}
		levels = append(levels, next)
	}
	sort.Strings(out)
	return out, nil
}
