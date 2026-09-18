package maven

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	baseVersionRe = regexp.MustCompile(`^\d+(\.\d+)*$`)
	versionLikeRe = regexp.MustCompile(`^\d+(\.\d+)*(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)
	qualTokenRe   = regexp.MustCompile(`\d+|[a-zA-Z]+`)
)

func LooksLikeVersion(s string) bool {
	return versionLikeRe.MatchString(s)
}

func ParseVersion(s string) (segs []int, qualifier string, err error) {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	base := s
	if i := strings.Index(base, "-"); i >= 0 {
		base, qualifier = base[:i], base[i+1:]
		if qualifier == "" {
			return nil, "", fmt.Errorf("invalid version %q: empty qualifier", s)
		}
	}
	if !baseVersionRe.MatchString(base) {
		return nil, "", fmt.Errorf("invalid version %q", s)
	}
	for _, tok := range strings.Split(base, ".") {
		n, err := strconv.Atoi(tok)
		if err != nil {
			return nil, "", fmt.Errorf("invalid version segment %q in %q", tok, s)
		}
		segs = append(segs, n)
	}
	return segs, qualifier, nil
}

func TwoSegment(s string) string {
	segs, _, err := ParseVersion(s)
	if err != nil {
		return ""
	}
	if len(segs) == 1 {
		return strconv.Itoa(segs[0])
	}
	return fmt.Sprintf("%d.%d", segs[0], segs[1])
}

func CompareVersions(a, b string) int {
	sa, qa, ea := ParseVersion(a)
	sb, qb, eb := ParseVersion(b)
	switch {
	case ea != nil && eb != nil:
		return strings.Compare(a, b)
	case ea != nil:
		return -1
	case eb != nil:
		return 1
	}
	for i := 0; i < len(sa) || i < len(sb); i++ {
		var va, vb int
		if i < len(sa) {
			va = sa[i]
		}
		if i < len(sb) {
			vb = sb[i]
		}
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	switch {
	case qa == "" && qb == "":
		return 0
	case qa == "":
		return 1
	case qb == "":
		return -1
	}
	return compareQualifiers(qa, qb)
}

func compareQualifiers(a, b string) int {
	ta := qualTokenRe.FindAllString(a, -1)
	tb := qualTokenRe.FindAllString(b, -1)
	for i := 0; i < len(ta) || i < len(tb); i++ {
		if i >= len(ta) {
			return -1
		}
		if i >= len(tb) {
			return 1
		}
		na, errA := strconv.Atoi(ta[i])
		nb, errB := strconv.Atoi(tb[i])
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		default:
			if c := strings.Compare(ta[i], tb[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}
