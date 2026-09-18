package maven

import (
	"os"
	"path/filepath"
	"testing"

	"barista/internal/output"
)

func fakeHome(t *testing.T, mvnScript string, jars ...string) string {
	t.Helper()
	home := t.TempDir()
	if mvnScript != "" {
		p := filepath.Join(home, "bin", mvnScript)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("script"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, j := range jars {
		p := filepath.Join(home, "lib", j)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("jar"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestProbe(t *testing.T) {
	home := fakeHome(t, "mvn", "maven-core-3.9.11.jar", "maven-model-3.9.11.jar")
	info, e := Probe(home)
	if e != nil {
		t.Fatalf("Probe: %v", e)
	}
	if info.Version != "3.9.11" || info.Home != home {
		t.Errorf("Probe = %+v, want version 3.9.11 home %s", info, home)
	}
}

func TestProbeCmdScript(t *testing.T) {
	home := fakeHome(t, "mvn.cmd", "maven-core-4.0.0-rc-4.jar")
	info, e := Probe(home)
	if e != nil || info.Version != "4.0.0-rc-4" {
		t.Errorf("Probe = %+v, %v; want 4.0.0-rc-4", info, e)
	}
}

func TestProbeNotAMaven(t *testing.T) {
	home := fakeHome(t, "", "maven-core-3.9.11.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeNotAMaven {
		t.Errorf("want NOT_A_MAVEN, got %v", e)
	}
}

func TestProbeNoCoreJar(t *testing.T) {
	home := fakeHome(t, "mvn")
	if _, e := Probe(home); e == nil || e.Code != output.CodeMavenProbeFailed {
		t.Errorf("want MAVEN_PROBE_FAILED, got %v", e)
	}
}

func TestProbeMultipleCoreJars(t *testing.T) {
	home := fakeHome(t, "mvn", "maven-core-3.9.9.jar", "maven-core-3.9.11.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeMavenProbeFailed {
		t.Errorf("want MAVEN_PROBE_FAILED, got %v", e)
	}
}

func TestProbeBadCoreVersion(t *testing.T) {
	home := fakeHome(t, "mvn", "maven-core-latest.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeMavenProbeFailed {
		t.Errorf("want MAVEN_PROBE_FAILED, got %v", e)
	}
}
