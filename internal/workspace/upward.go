package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

func FindUpward(start, boundary, name string) (string, bool) {
	return walkUpward(start, boundary, func(dir string) (string, bool) {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
		return "", false
	})
}

func FindGitRoot(start, boundary string) (string, bool) {
	return walkUpward(start, boundary, func(dir string) (string, bool) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		return "", false
	})
}

func walkUpward(start, boundary string, match func(dir string) (string, bool)) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	boundaryAbs := ""
	if boundary != "" {
		if b, err := filepath.Abs(boundary); err == nil {
			boundaryAbs = b
		}
	}
	for {
		if p, ok := match(dir); ok {
			return p, true
		}
		if boundaryAbs != "" && dir == boundaryAbs {
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		if boundaryAbs != "" && !isWithin(boundaryAbs, parent) {
			return "", false
		}
		dir = parent
	}
}

func isWithin(boundary, dir string) bool {
	rel, err := filepath.Rel(boundary, dir)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
