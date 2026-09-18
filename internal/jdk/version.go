package jdk

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var digitsRe = regexp.MustCompile(`\d+`)

func ParseVersion(s string) (major int, segs []int, err error) {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	if i := strings.IndexAny(s, "+-"); i >= 0 {
		s = s[:i]
	}
	toks := digitsRe.FindAllString(s, -1)
	if len(toks) > 1 && toks[0] == "1" {
		toks = toks[1:]
	}
	if len(toks) == 0 {
		return 0, nil, fmt.Errorf("no numeric version in %q", s)
	}
	segs = make([]int, len(toks))
	for i, t := range toks {
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid version segment %q in %q", t, s)
		}
		segs[i] = n
	}
	return segs[0], segs, nil
}

func CompareVersions(a, b string) int {
	_, sa, ea := ParseVersion(a)
	_, sb, eb := ParseVersion(b)
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
	return 0
}
