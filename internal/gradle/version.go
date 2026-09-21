package gradle

import (
	"strconv"
	"strings"

	"barista/internal/toolversion"
)

func LooksLikeVersion(s string) bool {
	return toolversion.LooksLike(s)
}

// ProbeVersion returns the version Probe reports for a distribution of the
// given release: qualified releases name their jars with the bare numeric
// base (gradle-8.11-milestone-1 ships gradle-core-8.11.jar), finals are
// returned unchanged.
func ProbeVersion(s string) string {
	segs, qualifier, err := toolversion.Parse(s)
	if err != nil || qualifier == "" {
		return s
	}
	parts := make([]string, len(segs))
	for i, n := range segs {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// NameFor returns the default registry name for a version: "gradle-" plus the
// full normalized version, qualifier included (8.14-rc-1 -> gradle-8.14-rc-1).
func NameFor(s string) string {
	segs, qualifier, err := toolversion.Parse(s)
	if err != nil {
		return ""
	}
	parts := make([]string, len(segs))
	for i, n := range segs {
		parts[i] = strconv.Itoa(n)
	}
	v := strings.Join(parts, ".")
	if qualifier != "" {
		v += "-" + qualifier
	}
	return "gradle-" + v
}
