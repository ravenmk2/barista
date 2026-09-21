package maven

import (
	"strconv"
	"strings"

	"barista/internal/toolversion"
)

func LooksLikeVersion(s string) bool {
	return toolversion.LooksLike(s)
}

func ParseVersion(s string) (segs []int, qualifier string, err error) {
	return toolversion.Parse(s)
}

// NameFor returns the default registry name for a version: "maven-" plus the
// full normalized version, qualifier included (4.0.0-rc-4 -> maven-4.0.0-rc-4).
func NameFor(s string) string {
	segs, qualifier, err := ParseVersion(s)
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
	return "maven-" + v
}

func CompareVersions(a, b string) int {
	return toolversion.Compare(a, b)
}
