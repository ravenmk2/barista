package node

import (
	"strings"

	"barista/internal/toolversion"
)

// LooksLikeVersion reports whether s parses as a node version (a leading v
// prefix is accepted).
func LooksLikeVersion(s string) bool {
	return toolversion.LooksLike(strings.TrimPrefix(s, "v"))
}

// NormalizeVersion strips an optional leading v and validates the rest as a
// dotted numeric version (v22.14.0 -> 22.14.0).
func NormalizeVersion(s string) (string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if _, _, err := toolversion.Parse(s); err != nil {
		return "", err
	}
	return s, nil
}

// NameFor returns the default registry name for a version: "node-" plus the
// normalized version (v22.14.0 -> node-22.14.0).
func NameFor(s string) string {
	v, err := NormalizeVersion(s)
	if err != nil {
		return ""
	}
	return "node-" + v
}

// ParseVersionFile extracts the version declared by a .node-version/.nvmrc
// file: the first non-empty, non-comment line, with an optional leading v.
func ParseVersionFile(data string) (string, bool) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		v, err := NormalizeVersion(line)
		if err != nil {
			return "", false
		}
		return v, true
	}
	return "", false
}
