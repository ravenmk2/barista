package gradlecli

import (
	"os"
	"path/filepath"

	"barista/internal/gradle"
	"barista/internal/workspace"
)

func wrapperVersion(cwd, wsRoot string) (version string, file string, ok bool) {
	boundary := wsRoot
	if gitRoot, found := workspace.FindGitRoot(cwd, wsRoot); found {
		boundary = gitRoot
	} else if wsRoot == "" {
		boundary = cwd
	}
	p, found := workspace.FindUpward(cwd, boundary, filepath.Join("gradle", "wrapper", "gradle-wrapper.properties"))
	if !found {
		return "", "", false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", "", false
	}
	v, ok := gradle.ExtractWrapperVersion(string(data))
	if !ok {
		return "", "", false
	}
	return v, p, true
}
