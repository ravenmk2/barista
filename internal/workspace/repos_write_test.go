package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeReposFile(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".barista")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "repos.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readReposFile(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".barista", "repos.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRelativizeURL(t *testing.T) {
	cases := []struct{ base, in, want string }{
		{"git@github.com:org/", "git@github.com:org/app.git", "app.git"},
		{"https://github.com/org", "https://github.com/org/app.git", "app.git"},
		{"", "https://github.com/org/app.git", "https://github.com/org/app.git"},
		{"https://github.com/org", "https://other.com/app.git", "https://other.com/app.git"},
		{"https://github.com/org", "https://github.com/org", "https://github.com/org"},
	}
	for _, c := range cases {
		if got := RelativizeURL(c.base, c.in); got != c.want {
			t.Errorf("RelativizeURL(%q, %q) = %q, want %q", c.base, c.in, got, c.want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	if NormalizeURL("https://x/app.git/") != "https://x/app" {
		t.Error("should strip .git and trailing slash")
	}
}

func TestAddRepoAppends(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, "{\n  \"baseUrl\": \"https://github.com/org\",\n  \"repos\": []\n}\n")
	_, added, err := AddRepo(root, "app", "https://github.com/org/app.git", "", []string{"java"})
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos.Repos) != 1 {
		t.Fatalf("want 1 repo, got %d", len(ws.Repos.Repos))
	}
	r := ws.Repos.Repos[0]
	if r.Name != "app" || r.URL != "app.git" || r.Path != "repos/app" || r.ResolvedURL != "https://github.com/org/app.git" {
		t.Errorf("unexpected entry: %+v resolved=%s", r, r.ResolvedURL)
	}
	if len(r.Labels) != 1 || r.Labels[0] != "java" {
		t.Errorf("labels lost: %v", r.Labels)
	}
}

func TestAddRepoPreservesUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{
  "baseUrl": "https://github.com/org",
  "x-custom": {"nested": true},
  "repos": [
    {"name": "old", "url": "old.git", "x-note": "keep me", "properties": {"jdk": "17"}}
  ]
}`)
	if _, _, err := AddRepo(root, "app", "app.git", "services/app", nil); err != nil {
		t.Fatal(err)
	}
	data := readReposFile(t, root)
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["x-custom"]; !ok {
		t.Error("unknown top-level key x-custom lost")
	}
	var repos []map[string]json.RawMessage
	if err := json.Unmarshal(top["repos"], &repos); err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(repos))
	}
	if _, ok := repos[0]["x-note"]; !ok {
		t.Error("unknown repo field x-note lost")
	}
	if _, ok := repos[0]["properties"]; !ok {
		t.Error("existing repo properties lost")
	}
}

func TestAddRepoIdempotentSkip(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{"baseUrl": "https://github.com/org", "repos": []}`)
	if _, _, err := AddRepo(root, "app", "https://github.com/org/app.git", "", nil); err != nil {
		t.Fatal(err)
	}
	_, added, err := AddRepo(root, "app", "app.git", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("second add of identical entry should report not-added")
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos.Repos) != 1 {
		t.Errorf("want 1 repo after re-add, got %d", len(ws.Repos.Repos))
	}
}

func TestAddRepoNameConflict(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{"repos": [{"name": "app", "url": "https://a.com/app.git"}]}`)
	_, _, err := AddRepo(root, "app", "https://b.com/app.git", "", nil)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	le, ok := err.(*LoadError)
	if !ok || le.Code != "REPO_EXISTS" {
		t.Errorf("want REPO_EXISTS LoadError, got %v", err)
	}
}

func TestAddRepoRelativeWithoutBaseURL(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{"repos": []}`)
	_, _, err := AddRepo(root, "app", "app.git", "", nil)
	if err == nil {
		t.Fatal("expected error for relative url without baseUrl")
	}
	le, ok := err.(*LoadError)
	if !ok || le.Code != "CONFIG_ERROR" {
		t.Errorf("want CONFIG_ERROR LoadError, got %v", err)
	}
}

func TestAddRepoBadPath(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{"repos": []}`)
	if _, _, err := AddRepo(root, "app", "https://a.com/app.git", "../escape", nil); err == nil {
		t.Fatal("expected error for escaping path")
	}
	if _, _, err := AddRepo(root, "app", "https://a.com/app.git", "/abs", nil); err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestAddRepoNormalizesURLComparison(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{"repos": [{"name": "app", "url": "https://a.com/app"}]}`)
	_, added, err := AddRepo(root, "app", "https://a.com/app.git", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("url differing only by .git suffix should be treated as identical")
	}
	if !strings.Contains(readReposFile(t, root), `"https://a.com/app"`) {
		t.Error("existing entry should be untouched")
	}
}

func TestAddRepoAbsLocalPath(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{"repos": []}`)
	_, added, err := AddRepo(root, "app", `C:/git/app.git`, "", nil)
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	r := ws.Repos.Repos[0]
	if r.URL != `C:/git/app.git` || r.ResolvedURL != `C:/git/app.git` {
		t.Errorf("abs path must be stored and resolved verbatim, got url=%q resolved=%q", r.URL, r.ResolvedURL)
	}
}

func TestRemoveRepoDropsEntry(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{
  "baseUrl": "https://github.com/org",
  "x-custom": {"nested": true},
  "repos": [
    {"name": "a", "url": "a.git", "x-note": "keep me"},
    {"name": "b", "url": "b.git", "properties": {"jdk": "17"}},
    {"name": "c", "url": "c.git"}
  ]
}`)
	repo, removed, err := RemoveRepo(root, "b")
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if repo.Name != "b" || repo.URL != "b.git" {
		t.Errorf("unexpected removed entry: %+v", repo)
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos.Repos) != 2 || ws.Repos.Repos[0].Name != "a" || ws.Repos.Repos[1].Name != "c" {
		t.Fatalf("unexpected remaining repos: %+v", ws.Repos.Repos)
	}
	data := readReposFile(t, root)
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["x-custom"]; !ok {
		t.Error("unknown top-level key x-custom lost")
	}
	if !strings.Contains(data, "x-note") {
		t.Error("unknown field of remaining entry lost")
	}
}

func TestRemoveRepoNotFound(t *testing.T) {
	root := t.TempDir()
	before := `{"repos": [{"name": "a", "url": "https://a.com/a.git"}]}`
	writeReposFile(t, root, before)
	_, removed, err := RemoveRepo(root, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("removing an unknown name should report not-removed")
	}
	if got := readReposFile(t, root); got != before {
		t.Error("file must be untouched when the name is absent")
	}
}

func TestRemoveRepoInvalidJSON(t *testing.T) {
	root := t.TempDir()
	writeReposFile(t, root, `{invalid`)
	_, _, err := RemoveRepo(root, "a")
	le, ok := err.(*LoadError)
	if !ok || le.Code != "CONFIG_ERROR" {
		t.Errorf("want CONFIG_ERROR LoadError, got %v", err)
	}
}
