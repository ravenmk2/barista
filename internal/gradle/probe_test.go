package gradle

import (
	"os"
	"path/filepath"
	"testing"

	"barista/internal/output"
)

func fakeHome(t *testing.T, scripts []string, jars ...string) string {
	t.Helper()
	home := t.TempDir()
	for _, s := range scripts {
		p := filepath.Join(home, "bin", s)
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

func distJars(version string) []string {
	return []string{
		"gradle-core-" + version + ".jar",
		"gradle-core-api-" + version + ".jar",
		"gradle-launcher-" + version + ".jar",
	}
}

func TestProbe(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"}, append(distJars("8.10.2"), "gradle-logging-8.10.2.jar")...)
	info, e := Probe(home)
	if e != nil {
		t.Fatalf("Probe: %v", e)
	}
	if info.Version != "8.10.2" || info.Home != home {
		t.Errorf("Probe = %+v, want version 8.10.2 home %s", info, home)
	}
}

func TestProbeBatScript(t *testing.T) {
	home := fakeHome(t, []string{"gradle.bat"}, distJars("9.0.0")...)
	info, e := Probe(home)
	if e != nil || info.Version != "9.0.0" {
		t.Errorf("Probe = %+v, %v; want base version 9.0.0 from a prerelease layout", info, e)
	}
}

func TestProbeNotAGradle(t *testing.T) {
	home := fakeHome(t, nil, "gradle-core-8.10.2.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeNotAGradle {
		t.Errorf("want NOT_A_GRADLE, got %v", e)
	}
}

func TestProbeNoCoreJar(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"})
	if _, e := Probe(home); e == nil || e.Code != output.CodeGradleProbeFailed {
		t.Errorf("want GRADLE_PROBE_FAILED, got %v", e)
	}
}

func TestProbeLauncherFallback(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"}, "gradle-launcher-8.9.jar")
	info, e := Probe(home)
	if e != nil || info.Version != "8.9" {
		t.Errorf("Probe = %+v, %v; want 8.9", info, e)
	}
}

func TestProbeCoreAndLauncherAgree(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"}, "gradle-core-8.9.jar", "gradle-launcher-8.9.jar")
	info, e := Probe(home)
	if e != nil || info.Version != "8.9" {
		t.Errorf("Probe = %+v, %v; want 8.9", info, e)
	}
}

func TestProbeCoreLauncherMismatch(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"}, "gradle-core-8.9.jar", "gradle-launcher-8.10.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeGradleProbeFailed {
		t.Errorf("want GRADLE_PROBE_FAILED, got %v", e)
	}
}

func TestProbeMultipleCoreJars(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"}, "gradle-core-8.9.jar", "gradle-core-8.10.2.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeGradleProbeFailed {
		t.Errorf("want GRADLE_PROBE_FAILED, got %v", e)
	}
}

func TestProbeUnparseableCoreJarIgnored(t *testing.T) {
	home := fakeHome(t, []string{"gradle", "gradle.bat"}, "gradle-core-latest.jar")
	if _, e := Probe(home); e == nil || e.Code != output.CodeGradleProbeFailed {
		t.Errorf("want GRADLE_PROBE_FAILED, got %v", e)
	}
}
