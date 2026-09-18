package jdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersionLine(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"temurin", "openjdk version \"17.0.7\" 2023-04-18\nOpenJDK Runtime Environment Temurin-17.0.7+7 (build 17.0.7+7)", "17.0.7"},
		{"oracle", "java version \"1.8.0_321\"\nJava(TM) SE Runtime Environment (build 1.8.0_321-b07)", "1.8.0_321"},
		{"quoted fallback", "some runtime \"21.0.3\" here", "21.0.3"},
		{"none", "no version anywhere", ""},
	}
	for _, tc := range cases {
		if got := parseVersionLine(tc.text); got != tc.want {
			t.Errorf("%s: parseVersionLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPropsValue(t *testing.T) {
	text := `Property settings:
    java.home = /opt/jdk-17
    java.vendor = Eclipse Adoptium
    java.version = 17.0.7
    java.vm.name = OpenJDK 64-Bit Server VM
`
	if got := propsValue(text, "java.version"); got != "17.0.7" {
		t.Errorf("java.version = %q, want 17.0.7", got)
	}
	if got := propsValue(text, "java.missing"); got != "" {
		t.Errorf("missing key = %q, want empty", got)
	}
}

func TestDetectDistro(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"temurin", "openjdk version \"17.0.7\"\nOpenJDK Runtime Environment Temurin-17.0.7+7 (build 17.0.7+7)", "temurin"},
		{"adoptium vendor", "java.vendor = Eclipse Adoptium\nopenjdk version \"17.0.7\"", "temurin"},
		{"corretto", "openjdk version \"11.0.19\"\nOpenJDK Runtime Environment Corretto-11.0.19.7.1 (build 11.0.19+7-LTS)", "corretto"},
		{"zulu", "openjdk version \"21.0.3\"\nOpenJDK Runtime Environment Zulu21.34.19-CA-jdk21.0.3 (build 21.0.3+9-LTS)", "zulu"},
		{"oracle", "java version \"1.8.0_321\"\nJava(TM) SE Runtime Environment (build 1.8.0_321-b07)\nJava HotSpot(TM) 64-Bit Server VM", "oracle"},
		{"plain openjdk", "openjdk version \"17.0.8\"\nOpenJDK Runtime Environment (build 17.0.8+7-Debian-1deb12u1)", "openjdk"},
		{"unknown falls back to temurin", "some exotic runtime \"21\"", "temurin"},
	}
	for _, tc := range cases {
		if got := detectDistro(tc.text); got != tc.want {
			t.Errorf("%s: detectDistro = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveHome(t *testing.T) {
	root := t.TempDir()
	if _, ok := resolveHome(root); ok {
		t.Error("empty dir should not resolve")
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", exeName("java")), []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolveHome(root); !ok || got != root {
		t.Errorf("resolveHome(root) = %q, %v", got, ok)
	}

	mac := t.TempDir()
	home := filepath.Join(mac, "Contents", "Home")
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", exeName("java")), []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolveHome(mac); !ok || got != home {
		t.Errorf("resolveHome(.jdk dir) = %q, %v; want %q", got, ok, home)
	}
}
