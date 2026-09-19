package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RelativizeURL strips the baseUrl prefix from u so the manifest stays
// portable; returns u unchanged when baseUrl is empty or not a prefix.
func RelativizeURL(base, u string) string {
	if base == "" {
		return u
	}
	prefix := strings.TrimRight(base, "/") + "/"
	if rest, ok := strings.CutPrefix(u, prefix); ok && rest != "" {
		return rest
	}
	return u
}

// NormalizeURL light-normalizes a clone URL for equality comparison.
func NormalizeURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, ".git")
	return u
}

// AddRepo appends a repo entry to <root>/.barista/repos.json, preserving
// unknown top-level keys and the raw bytes of existing entries. Returns the
// stored entry; appended is false when an entry with the same name and an
// equivalent URL already exists.
func AddRepo(root, name, url, path string, labels []string) (repo Repo, appended bool, err error) {
	p := filepath.Join(root, ".barista", "repos.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(p), err)}
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: %v", filepath.ToSlash(p), err)}
	}
	var baseURL string
	if raw, ok := top["baseUrl"]; ok {
		if err := json.Unmarshal(raw, &baseURL); err != nil {
			return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: baseUrl must be a string", filepath.ToSlash(p))}
		}
	}
	url = RelativizeURL(baseURL, url)
	if !strings.Contains(url, "://") && !strings.HasPrefix(url, "git@") && baseURL == "" {
		return repo, false, &LoadError{
			Code:    "CONFIG_ERROR",
			Message: fmt.Sprintf("relative url %q requires baseUrl in .barista/repos.json", url),
			Hint:    "pass an absolute URL or set baseUrl first",
		}
	}
	var entries []json.RawMessage
	if raw, ok := top["repos"]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("invalid %s: repos must be an array", filepath.ToSlash(p))}
		}
	}
	for _, raw := range entries {
		var existing Repo
		if err := json.Unmarshal(raw, &existing); err != nil || existing.Name == "" {
			continue
		}
		if existing.Name != name {
			continue
		}
		if NormalizeURL(existing.URL) == NormalizeURL(url) {
			existing.Path, _ = cleanRelPath(existing.Path, existing.Name)
			return existing, false, nil
		}
		return repo, false, &LoadError{
			Code:    "REPO_EXISTS",
			Message: fmt.Sprintf("repo %q is already registered with a different url (%s)", name, existing.URL),
			Hint:    "choose another --name or edit .barista/repos.json",
		}
	}
	clean, err := cleanRelPath(path, name)
	if err != nil {
		return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: err.Error()}
	}
	repo = Repo{Name: name, URL: url, Path: clean, Labels: labels}
	raw, err := json.Marshal(repo)
	if err != nil {
		return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot encode repo entry: %v", err)}
	}
	entries = append(entries, raw)
	arr, err := json.Marshal(entries)
	if err != nil {
		return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot encode %s: %v", filepath.ToSlash(p), err)}
	}
	top["repos"] = arr
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return repo, false, &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot encode %s: %v", filepath.ToSlash(p), err)}
	}
	out = append(out, '\n')
	if err := writeFileAtomic(p, out); err != nil {
		return repo, false, err
	}
	return repo, true, nil
}

func writeFileAtomic(path string, out []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot create %s: %v", filepath.ToSlash(filepath.Dir(path)), err)}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
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
