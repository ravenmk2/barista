package mvncli

import (
	"os"
	"path/filepath"

	"barista/internal/maven"
)

func wrapperVersion(cwd string, args []string) (version string, file string, ok bool) {
	p := filepath.Join(findBasedir(cwd, args), ".mvn", "wrapper", "maven-wrapper.properties")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", "", false
	}
	v, ok := maven.ExtractWrapperVersion(string(data))
	if !ok {
		return "", "", false
	}
	return v, p, true
}
