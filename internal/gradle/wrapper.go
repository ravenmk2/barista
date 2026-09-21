package gradle

import (
	"regexp"
	"strings"
)

var wrapperURLVersion = regexp.MustCompile(`gradle-(\d+(?:\.\d+)+(?:-[0-9A-Za-z][0-9A-Za-z.+-]*)?)-(?:bin|all)\.zip`)

func ExtractWrapperVersion(content string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "distributionUrl" {
			continue
		}
		m := wrapperURLVersion.FindStringSubmatch(strings.TrimSpace(value))
		if m == nil {
			return "", false
		}
		return m[1], true
	}
	return "", false
}
