package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func setUserHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestLoadUserConfigMissing(t *testing.T) {
	setUserHome(t)
	cf, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cf.Parallel != 0 || cf.Color != "" {
		t.Errorf("want zero ConfigFile, got %+v", cf)
	}
}

func TestLoadUserConfig(t *testing.T) {
	home := setUserHome(t)
	dir := filepath.Join(home, ".barista")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"parallel":20,"color":"never"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cf, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cf.Parallel != 20 || cf.Color != "never" {
		t.Errorf("got %+v, want {20 never}", cf)
	}
}

func TestLoadUserConfigSyntaxError(t *testing.T) {
	home := setUserHome(t)
	dir := filepath.Join(home, ".barista")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{ broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadUserConfig()
	if err == nil {
		t.Fatal("want error for broken JSON, got nil")
	}
	le, ok := err.(*LoadError)
	if !ok || le.Code != "CONFIG_ERROR" {
		t.Fatalf("want CONFIG_ERROR, got %v", err)
	}
	if !strings.Contains(le.Message, ".barista/config.json") {
		t.Errorf("error should name the file, got: %s", le.Message)
	}
}

func TestLoadUserConfigInvalidValue(t *testing.T) {
	home := setUserHome(t)
	dir := filepath.Join(home, ".barista")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"color":"rainbow"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUserConfig(); err == nil {
		t.Fatal("want error for invalid color, got nil")
	}
}

func TestMergeConfig(t *testing.T) {
	cases := []struct {
		name string
		user ConfigFile
		ws   ConfigFile
		want ConfigFile
	}{
		{"workspace overrides user", ConfigFile{Parallel: 20, Color: "never"}, ConfigFile{Parallel: 10}, ConfigFile{Parallel: 10, Color: "never"}},
		{"user fills gaps", ConfigFile{Parallel: 20, Color: "never"}, ConfigFile{Color: "always"}, ConfigFile{Parallel: 20, Color: "always"}},
		{"workspace only", ConfigFile{}, ConfigFile{Parallel: 4, Color: "auto"}, ConfigFile{Parallel: 4, Color: "auto"}},
		{"user only", ConfigFile{Parallel: 8}, ConfigFile{}, ConfigFile{Parallel: 8}},
		{"both empty", ConfigFile{}, ConfigFile{}, ConfigFile{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MergeConfig(tc.user, tc.ws); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MergeConfig(%+v, %+v) = %+v, want %+v", tc.user, tc.ws, got, tc.want)
			}
		})
	}
}

func TestMatchRepo(t *testing.T) {
	root := t.TempDir()
	ws := &Workspace{Root: root, Repos: &ReposFile{Repos: []Repo{
		{Name: "app", Path: "repos/app"},
		{Name: "app2", Path: "repos/app2"},
		{Name: "nested", Path: "repos/app/modules/nested"},
	}}}

	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"inside repo", filepath.Join(root, "repos", "app", "src"), "app"},
		{"repo root itself", filepath.Join(root, "repos", "app2"), "app2"},
		{"nested repo wins", filepath.Join(root, "repos", "app", "modules", "nested", "src"), "nested"},
		{"common-prefix sibling is not a match", filepath.Join(root, "repos", "app2x"), ""},
		{"workspace root matches nothing", root, ""},
		{"outside workspace", filepath.Dir(root), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ws.MatchRepo(c.dir)
			if c.want == "" {
				if got != nil {
					t.Errorf("want nil, got %q", got.Name)
				}
				return
			}
			if got == nil || got.Name != c.want {
				t.Errorf("want %q, got %+v", c.want, got)
			}
		})
	}
}

func TestFindWorkspaceRootHomeGuard(t *testing.T) {
	home := setUserHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".barista"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FindWorkspaceRoot(home); got != "" {
		t.Errorf("home dir must not be treated as workspace, got %q", got)
	}

	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".barista"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FindWorkspaceRoot(filepath.Join(ws, "sub", "dir")); got != ws {
		t.Errorf("want %q, got %q", ws, got)
	}
}
