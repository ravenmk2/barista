package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Repo struct {
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Path          string   `json:"path"`
	DefaultBranch string   `json:"defaultBranch,omitempty"`
	Labels        []string `json:"labels,omitempty"`

	ResolvedURL string `json:"-"`
}

type ReposFile struct {
	BaseURL       string         `json:"baseUrl"`
	DefaultBranch string         `json:"defaultBranch"`
	Config        map[string]any `json:"config"`
	Repos         []Repo         `json:"repos"`
}

func (f *ReposFile) ConfigBool(key string) (bool, bool) {
	v, ok := f.Config[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

type ConfigFile struct {
	Parallel int    `json:"parallel"`
	Color    string `json:"color"`
}

type Workspace struct {
	Root  string
	Repos *ReposFile
	Cfg   ConfigFile
}

func (w *Workspace) AbsPath(repo Repo) string {
	return filepath.Join(w.Root, filepath.FromSlash(repo.Path))
}

type LoadError struct {
	Code    string
	Message string
	Hint    string
}

func (e *LoadError) Error() string { return e.Message }

func Load(start string) (*Workspace, error) {
	root, err := findRoot(start)
	if err != nil {
		return nil, err
	}
	ws := &Workspace{Root: root}
	if err := ws.loadRepos(); err != nil {
		return nil, err
	}
	if err := ws.loadConfig(); err != nil {
		return nil, err
	}
	return ws, nil
}

func FindRoot(start string) (string, error) {
	return findRoot(start)
}

func findRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", &LoadError{Code: "WORKSPACE_NOT_FOUND", Message: err.Error()}
	}
	for {
		fi, err := os.Stat(filepath.Join(dir, ".barista"))
		if err == nil && fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", &LoadError{
				Code:    "WORKSPACE_NOT_FOUND",
				Message: "no .barista directory found in current directory or any parent",
				Hint:    "run inside a barista workspace or create .barista/repos.json",
			}
		}
		dir = parent
	}
}

func (w *Workspace) loadRepos() error {
	data, err := os.ReadFile(filepath.Join(w.Root, ".barista", "repos.json"))
	if err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read .barista/repos.json: %v", err)}
	}
	var rf ReposFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid .barista/repos.json: %v", err)}
	}
	seen := make(map[string]bool)
	for i := range rf.Repos {
		r := &rf.Repos[i]
		if r.Name == "" {
			return &LoadError{Code: "CONFIG_ERROR", Message: "repos.json: repo entry missing required field \"name\""}
		}
		if seen[r.Name] {
			return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("repos.json: duplicate repo name %q", r.Name)}
		}
		seen[r.Name] = true
		if r.URL == "" {
			return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("repos.json: repo %q missing required field \"url\"", r.Name)}
		}
		p, err := cleanRelPath(r.Path, r.Name)
		if err != nil {
			return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("repos.json: repo %q: %v", r.Name, err)}
		}
		r.Path = p
		r.ResolvedURL = ResolveURL(rf.BaseURL, r.URL)
	}
	w.Repos = &rf
	return nil
}

func cleanRelPath(p, name string) (string, error) {
	if p == "" {
		p = "repos/" + name
	}
	p = filepath.ToSlash(p)
	if path.IsAbs(p) || filepath.IsAbs(p) || (len(p) >= 2 && p[1] == ':') {
		return "", fmt.Errorf("path %q must be relative", p)
	}
	c := path.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("path %q escapes the workspace root", p)
	}
	return c, nil
}

func ResolveURL(base, u string) string {
	if strings.Contains(u, "://") || strings.HasPrefix(u, "git@") {
		return u
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/")
}

func (w *Workspace) loadConfig() error {
	data, err := os.ReadFile(filepath.Join(w.Root, ".barista", "config.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read .barista/config.json: %v", err)}
	}
	var cf ConfigFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid .barista/config.json: %v", err)}
	}
	switch cf.Color {
	case "", "auto", "always", "never":
	default:
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("config.json: invalid color %q (want auto|always|never)", cf.Color)}
	}
	if cf.Parallel < 0 {
		return &LoadError{Code: "CONFIG_ERROR", Message: "config.json: parallel must be >= 0"}
	}
	w.Cfg = cf
	return nil
}
