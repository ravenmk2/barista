package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteJavaVersionFile(dir, version string) (previous string, err error) {
	p := filepath.Join(dir, ".java-version")
	data, err := os.ReadFile(p)
	switch {
	case err == nil:
		previous = strings.TrimSpace(string(data))
	case os.IsNotExist(err):
	default:
		return "", &LoadError{Code: "CONFIG_ERROR", Message: fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(p), err)}
	}
	if previous == version {
		return previous, nil
	}
	if err := writeFileAtomic(p, []byte(version+"\n")); err != nil {
		return "", err
	}
	return previous, nil
}
