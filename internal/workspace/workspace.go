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
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Path          string            `json:"path"`
	DefaultBranch string            `json:"defaultBranch,omitempty"`
	Labels        []string          `json:"labels,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`

	ResolvedURL string `json:"-"`
}

func (r *Repo) Property(key string) (string, bool) {
	if r.Properties == nil {
		return "", false
	}
	v, ok := r.Properties[key]
	return v, ok
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
	Parallel        int            `json:"parallel"`
	Color           string         `json:"color"`
	InstallDir      string         `json:"installDir"`
	MavenInstallDir string         `json:"mavenInstallDir"`
	Properties      map[string]any `json:"properties"`
}

func (f ConfigFile) Property(key string) (string, bool) {
	v, ok := f.Properties[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
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

func FindWorkspaceRoot(start string) string {
	root, err := findRoot(start)
	if err != nil {
		return ""
	}
	if home, herr := os.UserHomeDir(); herr == nil && root == filepath.Clean(home) {
		return ""
	}
	return root
}

func (w *Workspace) MatchRepo(dir string) *Repo {
	var best *Repo
	bestLen := -1
	for i := range w.Repos.Repos {
		abs := w.AbsPath(w.Repos.Repos[i])
		rel, err := filepath.Rel(abs, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(abs) > bestLen {
			best = &w.Repos.Repos[i]
			bestLen = len(abs)
		}
	}
	return best
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
	cf, err := parseConfig(data, ".barista/config.json")
	if err != nil {
		return err
	}
	w.Cfg = cf
	return nil
}

func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".barista", "config.json"), nil
}

func LoadUserConfig() (ConfigFile, error) {
	p, err := UserConfigPath()
	if err != nil {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: err.Error()}
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return ConfigFile{}, nil
	}
	if err != nil {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(p), err)}
	}
	return parseConfig(data, filepath.ToSlash(p))
}

func parseConfig(data []byte, source string) (ConfigFile, error) {
	var cf ConfigFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: %v", source, err)}
	}
	switch cf.Color {
	case "", "auto", "always", "never":
	default:
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: invalid color %q (want auto|always|never)", source, cf.Color)}
	}
	if cf.Parallel < 0 {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: parallel must be >= 0", source)}
	}
	if cf.InstallDir != "" {
		cf.InstallDir = ExpandHome(cf.InstallDir)
		if !filepath.IsAbs(cf.InstallDir) {
			return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: installDir must be an absolute path (~ allowed)", source)}
		}
	}
	if cf.MavenInstallDir != "" {
		cf.MavenInstallDir = ExpandHome(cf.MavenInstallDir)
		if !filepath.IsAbs(cf.MavenInstallDir) {
			return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: mavenInstallDir must be an absolute path (~ allowed)", source)}
		}
	}
	return cf, nil
}

func LoadConfigFile(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ConfigFile{}, nil
	}
	if err != nil {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(path), err)}
	}
	return parseConfig(data, filepath.ToSlash(path))
}

func SetConfigProperty(path, key, value string) error {
	doc := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(path), err)}
	default:
		if err := json.Unmarshal(data, &doc); err != nil {
			return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: %v", filepath.ToSlash(path), err)}
		}
	}
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		props = map[string]any{}
		doc["properties"] = props
	}
	props[key] = value
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot encode %s: %v", filepath.ToSlash(path), err)}
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot create %s: %v", filepath.ToSlash(filepath.Dir(path)), err)}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "config.json.*")
	if err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot write %s: %v", filepath.ToSlash(path), err)}
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot write %s: %v", filepath.ToSlash(path), err)}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot write %s: %v", filepath.ToSlash(path), err)}
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(path)
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Remove(tmpName)
			return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot write %s: %v", filepath.ToSlash(path), err)}
		}
	}
	return nil
}

func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

func MergeConfig(user, ws ConfigFile) ConfigFile {
	out := user
	if ws.Color != "" {
		out.Color = ws.Color
	}
	if ws.Parallel != 0 {
		out.Parallel = ws.Parallel
	}
	return out
}
