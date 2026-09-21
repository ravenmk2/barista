package comp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	return home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestJdkSpecs(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".barista", "jdk.json"),
		`{"jdks":[{"name":"temurin17","major":17,"version":"17.0.12","path":"/j/17"},{"name":"temurin21","major":21,"version":"21.0.5","path":"/j/21"}]}`)
	specs := JdkSpecs()
	joined := strings.Join(specs, "\n")
	for _, want := range []string{"temurin17\t17.0.12", "temurin21\t21.0.5", "17\ttemurin17 17.0.12", "21\ttemurin21 21.0.5"} {
		if !strings.Contains(joined, want) {
			t.Errorf("JdkSpecs missing %q in %v", want, specs)
		}
	}
	if len(specs) != 4 {
		t.Errorf("want 2 names + 2 majors, got %v", specs)
	}
}

func TestJdkSpecsBrokenRegistry(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".barista", "jdk.json"), `{ broken`)
	if got := JdkSpecs(); got != nil {
		t.Errorf("broken registry must yield no candidates, got %v", got)
	}
}

func TestMavenSpecs(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".barista", "maven.json"),
		`{"installations":[{"name":"maven-3.9.11","version":"3.9.11","path":"/m/3.9"}],"default":"maven-3.9.11"}`)
	joined := strings.Join(MavenSpecs(), "\n")
	if !strings.Contains(joined, "maven-3.9.11\t3.9.11") || !strings.Contains(joined, "3.9.11\tmaven-3.9.11") {
		t.Errorf("unexpected specs: %v", joined)
	}
}

func TestGradleSpecs(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".barista", "gradle.json"),
		`{"installations":[{"name":"gradle-8.10.2","version":"8.10.2","path":"/g/8.10"}],"default":"gradle-8.10.2"}`)
	joined := strings.Join(GradleSpecs(), "\n")
	if !strings.Contains(joined, "gradle-8.10.2\t8.10.2") || !strings.Contains(joined, "8.10.2\tgradle-8.10.2") {
		t.Errorf("unexpected specs: %v", joined)
	}
}

func TestGradleNames(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".barista", "gradle.json"),
		`{"installations":[{"name":"gradle-8.10.2","version":"8.10.2","path":"/g/8.10"}],"default":"gradle-8.10.2"}`)
	names := GradleNames()
	if len(names) != 1 || names[0] != "gradle-8.10.2\t8.10.2" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestGradleSpecsBrokenRegistry(t *testing.T) {
	home := setHome(t)
	writeFile(t, filepath.Join(home, ".barista", "gradle.json"), `{ broken`)
	if got := GradleSpecs(); got != nil {
		t.Errorf("broken registry must yield no candidates, got %v", got)
	}
	if got := GradleNames(); got != nil {
		t.Errorf("broken registry must yield no candidates, got %v", got)
	}
}

func TestRepoNamesAndLabels(t *testing.T) {
	setHome(t)
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, ".barista", "repos.json"),
		`{"repos":[{"name":"app","url":"app.git","path":"repos/app","labels":["java","svc"]},{"name":"web","url":"web.git","labels":["web"]}]}`)
	writeFile(t, filepath.Join(ws, ".barista", "properties.json"), `{}`)
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(ws); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(RepoNames(), "\n")
	if !strings.Contains(joined, "app\trepos/app") || !strings.Contains(joined, "web\t") {
		t.Errorf("unexpected repo names: %v", joined)
	}
	labels := Labels()
	if len(labels) != 3 {
		t.Errorf("want 3 unique labels, got %v", labels)
	}
}

func TestRepoNamesOutsideWorkspace(t *testing.T) {
	setHome(t)
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := RepoNames(); got != nil {
		t.Errorf("outside workspace must yield no candidates, got %v", got)
	}
}

func TestSchemaNames(t *testing.T) {
	names := SchemaNames()
	if len(names) != 7 {
		t.Fatalf("want 7 schema candidates, got %v", names)
	}
	if !strings.Contains(strings.Join(names, "\n"), "manifest\t") {
		t.Errorf("manifest missing in %v", names)
	}
}

func TestShells(t *testing.T) {
	if len(Shells()) == 0 {
		t.Error("shells must not be empty")
	}
}
