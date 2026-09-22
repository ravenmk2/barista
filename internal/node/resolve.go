package node

import (
	"fmt"
	"strings"

	"barista/internal/output"
	"barista/internal/toolversion"
)

func (r *Registry) Resolve(arg string) (*Entry, string, *output.ErrInfo) {
	if arg == "" {
		return r.ResolveDefault()
	}
	if e := r.Find(arg); e != nil {
		return e, "name", nil
	}
	if LooksLikeVersion(arg) {
		want := strings.TrimPrefix(arg, "v")
		for i := range r.Installations {
			if r.Installations[i].Version == want {
				return &r.Installations[i], "version", nil
			}
		}
		segs, _, _ := toolversion.Parse(want)
		for depth := len(segs); depth >= 1; depth-- {
			var best *Entry
			for i := range r.Installations {
				e := &r.Installations[i]
				es, _, err := toolversion.Parse(e.Version)
				if err != nil || len(es) < depth {
					continue
				}
				if !equalSegs(es[:depth], segs[:depth]) {
					continue
				}
				if best == nil || toolversion.Compare(e.Version, best.Version) > 0 {
					best = e
				}
			}
			if best != nil {
				return best, "prefix", nil
			}
		}
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeNodeNotFound,
		Message: fmt.Sprintf("no registered node matches %q", arg),
		Hint:    "run: barista node list",
	}
}

func (r *Registry) ResolveDefault() (*Entry, string, *output.ErrInfo) {
	if r.Default == "" {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeNodeNotFound,
			Message: "no default node configured",
			Hint:    "set one with: barista node set-default <name>",
		}
	}
	if e := r.Find(r.Default); e != nil {
		return e, "default", nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeNodeNotFound,
		Message: fmt.Sprintf("default node %q is not registered", r.Default),
		Hint:    "run: barista node list; fix with: barista node set-default <name>",
	}
}

func equalSegs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
