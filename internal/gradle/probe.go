package gradle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"barista/internal/output"
	"barista/internal/toolversion"
)

type Info struct {
	Home    string
	Version string
}

func Probe(path string) (Info, *output.ErrInfo) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, &output.ErrInfo{Code: output.CodeNotAGradle, Message: err.Error()}
	}
	if !hasGradle(abs) {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNotAGradle,
			Message: fmt.Sprintf("%s: no bin/gradle found (not a gradle home)", filepath.ToSlash(abs)),
		}
	}
	version, err := coreVersion(abs)
	if err != nil {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeGradleProbeFailed,
			Message: fmt.Sprintf("%s: %v", filepath.ToSlash(abs), err),
		}
	}
	return Info{Home: abs, Version: version}, nil
}

func hasGradle(home string) bool {
	for _, name := range []string{"gradle", "gradle.bat"} {
		fi, err := os.Stat(filepath.Join(home, "bin", name))
		if err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func coreVersion(home string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(home, "lib"))
	if err != nil {
		return "", err
	}
	core := matchJars(entries, "gradle-core-")
	launcher := matchJars(entries, "gradle-launcher-")
	if len(core) > 1 {
		return "", fmt.Errorf("multiple gradle-core jars: %s", strings.Join(core, ", "))
	}
	if len(launcher) > 1 {
		return "", fmt.Errorf("multiple gradle-launcher jars: %s", strings.Join(launcher, ", "))
	}
	if len(core) == 0 && len(launcher) == 0 {
		return "", fmt.Errorf("no lib/gradle-core-*.jar or lib/gradle-launcher-*.jar found")
	}
	v := ""
	if len(core) == 1 {
		v = jarVersion(core[0], "gradle-core-")
	}
	if len(launcher) == 1 {
		lv := jarVersion(launcher[0], "gradle-launcher-")
		if v != "" && v != lv {
			return "", fmt.Errorf("gradle-core is %s but gradle-launcher is %s", v, lv)
		}
		v = lv
	}
	return v, nil
}

func matchJars(entries []os.DirEntry, prefix string) []string {
	var found []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".jar") {
			continue
		}
		if _, _, err := toolversion.Parse(jarVersion(n, prefix)); err != nil {
			continue
		}
		found = append(found, n)
	}
	return found
}

func jarVersion(name, prefix string) string {
	return strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jar")
}
