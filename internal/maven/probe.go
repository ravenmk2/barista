package maven

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"barista/internal/output"
)

type Info struct {
	Home    string
	Version string
}

func Probe(path string) (Info, *output.ErrInfo) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, &output.ErrInfo{Code: output.CodeNotAMaven, Message: err.Error()}
	}
	if !hasMvn(abs) {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNotAMaven,
			Message: fmt.Sprintf("%s: no bin/mvn found (not a maven home)", filepath.ToSlash(abs)),
		}
	}
	version, err := coreVersion(abs)
	if err != nil {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeMavenProbeFailed,
			Message: fmt.Sprintf("%s: %v", filepath.ToSlash(abs), err),
		}
	}
	return Info{Home: abs, Version: version}, nil
}

func hasMvn(home string) bool {
	for _, name := range []string{"mvn", "mvn.cmd"} {
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
	var found []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "maven-core-") && strings.HasSuffix(n, ".jar") {
			found = append(found, n)
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no lib/maven-core-*.jar found")
	}
	if len(found) > 1 {
		return "", fmt.Errorf("multiple maven-core jars: %s", strings.Join(found, ", "))
	}
	v := strings.TrimSuffix(strings.TrimPrefix(found[0], "maven-core-"), ".jar")
	if _, _, err := ParseVersion(v); err != nil {
		return "", fmt.Errorf("cannot parse maven version from %q", found[0])
	}
	return v, nil
}
