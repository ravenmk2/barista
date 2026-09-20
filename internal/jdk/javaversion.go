package jdk

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var distroAlias = map[string]string{
	"tem": "temurin", "temurin": "temurin",
	"ms": "microsoft", "microsoft": "microsoft",
	"cor": "corretto", "corretto": "corretto",
	"zul": "zulu", "zulu": "zulu",
	"grl": "graalvm", "graalvm": "graalvm",
}

var (
	numPrefix    = regexp.MustCompile(`^\d+(?:\.\d+)*`)
	numAnywhere  = regexp.MustCompile(`\d+(?:\.\d+)*`)
	tokenDivider = func(r rune) bool { return r == '-' || r == '@' || unicode.IsSpace(r) }
)

func ParseJavaVersionFile(content string) (major int, distro string, ok bool) {
	s := strings.TrimSpace(content)
	if s == "" {
		return 0, "", false
	}
	num := ""
	for _, tok := range strings.FieldsFunc(s, tokenDivider) {
		if d, is := distroAlias[strings.ToLower(tok)]; is {
			if distro == "" {
				distro = d
			}
			continue
		}
		if num == "" {
			num = numPrefix.FindString(tok)
		}
	}
	if num == "" {
		num = numAnywhere.FindString(s)
	}
	if num == "" {
		return 0, "", false
	}
	segs := strings.Split(num, ".")
	lead := segs[0]
	if lead == "1" && len(segs) > 1 {
		lead = segs[1]
	}
	major, err := strconv.Atoi(lead)
	if err != nil || major < 1 {
		return 0, "", false
	}
	return major, distro, true
}

func ResolveJavaVersionSpec(reg *Registry, major int, distro string) (*Entry, string) {
	if distro != "" {
		name := distro + strconv.Itoa(major)
		if e := reg.Find(name); e != nil {
			return e, name
		}
	}
	spec := strconv.Itoa(major)
	entry, _, e := reg.Resolve(spec)
	if e != nil {
		return nil, spec
	}
	return entry, spec
}
