package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"barista/internal/download"
)

type Repo struct {
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Path          string            `json:"path"`
	DefaultBranch string            `json:"defaultBranch,omitempty"`
	Labels        []string          `json:"labels,omitempty"`
	Deps          []string          `json:"deps,omitempty"`
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
	BaseURL       string `json:"baseUrl"`
	DefaultBranch string `json:"defaultBranch"`
	Repos         []Repo `json:"repos"`
}

type ConfigFile struct {
	Parallel             int    `json:"parallel"`
	Color                string `json:"color"`
	ColorProfile         string `json:"colorProfile"`
	DownloadMirror       string `json:"download.mirror"`
	JDKDownloadMirror    string `json:"jdk.download.mirror"`
	MavenDownloadMirror  string `json:"maven.download.mirror"`
	GradleDownloadMirror string `json:"gradle.download.mirror"`
}

type Workspace struct {
	Root  string
	Repos *ReposFile
	Cfg   ConfigFile
	Props Properties
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
	props, err := LoadProperties(filepath.Join(root, ".barista", "properties.json"))
	if err != nil {
		return nil, err
	}
	ws.Props = props
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

// IsAbsURL reports whether u is a self-contained clone source: a scheme
// URL, an scp-style git@ URL, or an absolute local path (POSIX root,
// Windows drive, UNC).
func IsAbsURL(u string) bool {
	if strings.Contains(u, "://") || strings.HasPrefix(u, "git@") {
		return true
	}
	if strings.HasPrefix(u, `/`) || strings.HasPrefix(u, `\`) {
		return true
	}
	return len(u) >= 3 && u[1] == ':' && (u[2] == '/' || u[2] == '\\') &&
		(u[0] >= 'A' && u[0] <= 'Z' || u[0] >= 'a' && u[0] <= 'z')
}

func ResolveURL(base, u string) string {
	if IsAbsURL(u) {
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
	switch cf.ColorProfile {
	case "", "auto", "truecolor", "256", "16":
	default:
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: invalid colorProfile %q (want auto|truecolor|256|16)", source, cf.ColorProfile)}
	}
	if cf.Parallel < 0 {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: parallel must be >= 0", source)}
	}
	if err := download.ValidateGlobalMirror(cf.DownloadMirror); err != nil {
		return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: %v", source, err)}
	}
	for _, kv := range []struct {
		key, domain, value string
	}{
		{"jdk.download.mirror", download.DomainJDK, cf.JDKDownloadMirror},
		{"maven.download.mirror", download.DomainMaven, cf.MavenDownloadMirror},
		{"gradle.download.mirror", download.DomainGradle, cf.GradleDownloadMirror},
	} {
		if err := download.ValidateMirror(kv.domain, kv.value); err != nil {
			return ConfigFile{}, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("%s: %s: %v", source, kv.key, err)}
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
	if ws.ColorProfile != "" {
		out.ColorProfile = ws.ColorProfile
	}
	if ws.Parallel != 0 {
		out.Parallel = ws.Parallel
	}
	if ws.DownloadMirror != "" {
		out.DownloadMirror = ws.DownloadMirror
	}
	if ws.JDKDownloadMirror != "" {
		out.JDKDownloadMirror = ws.JDKDownloadMirror
	}
	if ws.MavenDownloadMirror != "" {
		out.MavenDownloadMirror = ws.MavenDownloadMirror
	}
	if ws.GradleDownloadMirror != "" {
		out.GradleDownloadMirror = ws.GradleDownloadMirror
	}
	return out
}

// LoadMergedConfig returns the user-level config overlaid with the
// workspace-level config when start is inside a workspace (workspace wins per
// key). Not being inside a workspace is not an error.
func LoadMergedConfig(start string) (ConfigFile, error) {
	user, err := LoadUserConfig()
	if err != nil {
		return ConfigFile{}, err
	}
	root := FindWorkspaceRoot(start)
	if root == "" {
		return user, nil
	}
	ws, err := LoadConfigFile(filepath.Join(root, ".barista", "config.json"))
	if err != nil {
		return ConfigFile{}, err
	}
	return MergeConfig(user, ws), nil
}

// MirrorValueFor returns the raw configured mirror value for domain: the
// per-domain key wins over the global download.mirror; "" when unset.
func (cf ConfigFile) MirrorValueFor(domain string) string {
	value := cf.DownloadMirror
	switch domain {
	case download.DomainJDK:
		if cf.JDKDownloadMirror != "" {
			value = cf.JDKDownloadMirror
		}
	case download.DomainMaven:
		if cf.MavenDownloadMirror != "" {
			value = cf.MavenDownloadMirror
		}
	case download.DomainGradle:
		if cf.GradleDownloadMirror != "" {
			value = cf.GradleDownloadMirror
		}
	}
	return value
}

// MirrorBaseFor resolves the effective download mirror base URL for domain;
// "" means the official source.
func (cf ConfigFile) MirrorBaseFor(domain string) (string, error) {
	return download.MirrorBase(domain, cf.MirrorValueFor(domain))
}
