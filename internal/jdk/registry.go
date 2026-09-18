package jdk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"barista/internal/output"
)

type Entry struct {
	Name    string `json:"name"`
	Major   int    `json:"major"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Managed bool   `json:"managed,omitempty"`
}

type Registry struct {
	JDKs     []Entry           `json:"jdks"`
	Defaults map[string]string `json:"defaults,omitempty"`
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
	return filepath.Join(home, ".barista", "jdk.json"), nil
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
	for i := range reg.JDKs {
		e := &reg.JDKs[i]
		if e.Name == "" {
			return nil, configError("%s: jdk entry missing required field \"name\"", filepath.ToSlash(path))
		}
		if seen[e.Name] {
			return nil, configError("%s: duplicate jdk name %q", filepath.ToSlash(path), e.Name)
		}
		seen[e.Name] = true
		if e.Major < 1 {
			return nil, configError("%s: jdk %q has invalid major %d", filepath.ToSlash(path), e.Name, e.Major)
		}
		if e.Path == "" {
			return nil, configError("%s: jdk %q missing required field \"path\"", filepath.ToSlash(path), e.Name)
		}
	}
	return reg, nil
}

func (r *Registry) Save(path string) *output.ErrInfo {
	if r.JDKs == nil {
		r.JDKs = []Entry{}
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
	tmp, err := os.CreateTemp(dir, "jdk.json.*")
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
	for i := range r.JDKs {
		if r.JDKs[i].Name == name {
			return &r.JDKs[i]
		}
	}
	return nil
}

func (r *Registry) Add(e Entry) *output.ErrInfo {
	if r.Find(e.Name) != nil {
		return &output.ErrInfo{
			Code:    output.CodeJDKExists,
			Message: fmt.Sprintf("JDK %q is already registered", e.Name),
			Hint:    "choose another name or run: barista jdk remove " + e.Name,
		}
	}
	r.JDKs = append(r.JDKs, e)
	return nil
}

func (r *Registry) Remove(name string) (*Entry, []int, *output.ErrInfo) {
	for i := range r.JDKs {
		if r.JDKs[i].Name == name {
			removed := r.JDKs[i]
			r.JDKs = append(r.JDKs[:i], r.JDKs[i+1:]...)
			var cleared []int
			for k, v := range r.Defaults {
				if v == name {
					delete(r.Defaults, k)
					if n, err := strconv.Atoi(k); err == nil {
						cleared = append(cleared, n)
					}
				}
			}
			sort.Ints(cleared)
			return &removed, cleared, nil
		}
	}
	return nil, nil, &output.ErrInfo{
		Code:    output.CodeJDKNotFound,
		Message: fmt.Sprintf("JDK %q is not registered", name),
	}
}

func (r *Registry) SetDefault(major int, name string) (*Entry, *output.ErrInfo) {
	e := r.Find(name)
	if e == nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeJDKNotFound,
			Message: fmt.Sprintf("JDK %q is not registered", name),
		}
	}
	if e.Major != major {
		return nil, &output.ErrInfo{
			Code:    output.CodeJDKMajorMismatch,
			Message: fmt.Sprintf("JDK %q is major version %d, cannot be the default for %d", name, e.Major, major),
		}
	}
	if r.Defaults == nil {
		r.Defaults = map[string]string{}
	}
	r.Defaults[strconv.Itoa(major)] = name
	return e, nil
}

func (r *Registry) Resolve(arg string) (*Entry, string, *output.ErrInfo) {
	if major, err := strconv.Atoi(arg); err == nil && major >= 1 {
		return r.resolveMajor(major)
	}
	if e := r.Find(arg); e != nil {
		return e, "name", nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeJDKNotFound,
		Message: fmt.Sprintf("JDK %q is not registered", arg),
		Hint:    "run: barista jdk list",
	}
}

func (r *Registry) resolveMajor(major int) (*Entry, string, *output.ErrInfo) {
	if name, ok := r.Defaults[strconv.Itoa(major)]; ok {
		if e := r.Find(name); e != nil && e.Major == major {
			return e, "default", nil
		}
	}
	var best *Entry
	for i := range r.JDKs {
		e := &r.JDKs[i]
		if e.Major != major {
			continue
		}
		if best == nil || CompareVersions(e.Version, best.Version) > 0 {
			best = e
		}
	}
	if best != nil {
		return best, "latest", nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeJDKNotFound,
		Message: fmt.Sprintf("no JDK registered for major version %d", major),
		Hint:    "register one with: barista jdk add <name> <path>; or run: barista jdk discover",
	}
}

func (r *Registry) DefaultMajors(name string) []int {
	out := []int{}
	for k, v := range r.Defaults {
		if v == name {
			if n, err := strconv.Atoi(k); err == nil {
				out = append(out, n)
			}
		}
	}
	sort.Ints(out)
	return out
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
