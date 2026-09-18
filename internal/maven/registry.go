package maven

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"barista/internal/output"
)

type Entry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Managed bool   `json:"managed,omitempty"`
}

type Registry struct {
	Installations []Entry `json:"installations"`
	Default       string  `json:"default,omitempty"`
	Jdk           string  `json:"jdk,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func ValidName(name string) bool {
	return namePattern.MatchString(name)
}

func RegistryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".barista", "maven.json"), nil
}

func Load(path string) (*Registry, *output.ErrInfo) {
	reg := &Registry{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return reg, nil
	}
	if err != nil {
		return nil, configError("cannot read %s: %v", filepath.ToSlash(path), err)
	}
	if err := json.Unmarshal(data, reg); err != nil {
		return nil, configError("invalid %s: %v", filepath.ToSlash(path), err)
	}
	seen := make(map[string]bool)
	for i := range reg.Installations {
		e := &reg.Installations[i]
		if e.Name == "" {
			return nil, configError("%s: maven entry missing required field \"name\"", filepath.ToSlash(path))
		}
		if seen[e.Name] {
			return nil, configError("%s: duplicate maven name %q", filepath.ToSlash(path), e.Name)
		}
		seen[e.Name] = true
		if e.Version == "" {
			return nil, configError("%s: maven %q missing required field \"version\"", filepath.ToSlash(path), e.Name)
		}
		if e.Path == "" {
			return nil, configError("%s: maven %q missing required field \"path\"", filepath.ToSlash(path), e.Name)
		}
	}
	return reg, nil
}

func (r *Registry) Save(path string) *output.ErrInfo {
	if r.Installations == nil {
		r.Installations = []Entry{}
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return configError("cannot encode registry: %v", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return configError("cannot create %s: %v", filepath.ToSlash(dir), err)
	}
	tmp, err := os.CreateTemp(dir, "maven.json.*")
	if err != nil {
		return configError("cannot write %s: %v", filepath.ToSlash(path), err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return configError("cannot write %s: %v", filepath.ToSlash(path), err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return configError("cannot write %s: %v", filepath.ToSlash(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(path)
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Remove(tmpName)
			return configError("cannot write %s: %v", filepath.ToSlash(path), err)
		}
	}
	return nil
}

func (r *Registry) Find(name string) *Entry {
	for i := range r.Installations {
		if r.Installations[i].Name == name {
			return &r.Installations[i]
		}
	}
	return nil
}

func (r *Registry) Add(e Entry) *output.ErrInfo {
	if r.Find(e.Name) != nil {
		return &output.ErrInfo{
			Code:    output.CodeMavenExists,
			Message: fmt.Sprintf("maven %q is already registered", e.Name),
			Hint:    "choose another name or run: barista maven remove " + e.Name,
		}
	}
	r.Installations = append(r.Installations, e)
	return nil
}

func (r *Registry) Remove(name string) (*Entry, bool, *output.ErrInfo) {
	for i := range r.Installations {
		if r.Installations[i].Name == name {
			removed := r.Installations[i]
			r.Installations = append(r.Installations[:i], r.Installations[i+1:]...)
			clearedDefault := r.Default == name
			if clearedDefault {
				r.Default = ""
			}
			return &removed, clearedDefault, nil
		}
	}
	return nil, false, &output.ErrInfo{
		Code:    output.CodeMavenNotFound,
		Message: fmt.Sprintf("maven %q is not registered", name),
	}
}

func (r *Registry) SetDefault(name string) (*Entry, *output.ErrInfo) {
	e := r.Find(name)
	if e == nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeMavenNotFound,
			Message: fmt.Sprintf("maven %q is not registered", name),
		}
	}
	r.Default = name
	return e, nil
}

func (r *Registry) SetJdk(spec string) {
	r.Jdk = spec
}

func (r *Registry) AvailableName(base string) string {
	if r.Find(base) == nil {
		return base
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("%s-%d", base, i)
		if r.Find(name) == nil {
			return name
		}
	}
}

func configError(format string, args ...any) *output.ErrInfo {
	return &output.ErrInfo{
		Code:    output.CodeConfigError,
		Message: fmt.Sprintf(format, args...),
	}
}
