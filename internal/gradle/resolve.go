package gradle

import (
	"fmt"

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
		for i := range r.Installations {
			if r.Installations[i].Version == arg {
				return &r.Installations[i], "version", nil
			}
		}
		segs, _, _ := toolversion.Parse(arg)
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
		Code:    output.CodeGradleNotFound,
		Message: fmt.Sprintf("no registered gradle matches %q", arg),
		Hint:    "run: barista gradle list",
	}
}

func (r *Registry) ResolveDefault() (*Entry, string, *output.ErrInfo) {
	if r.Default == "" {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeGradleNotFound,
			Message: "no default gradle configured",
			Hint:    "set one with: barista gradle set-default <name>",
		}
	}
	if e := r.Find(r.Default); e != nil {
		return e, "default", nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeGradleNotFound,
		Message: fmt.Sprintf("default gradle %q is not registered", r.Default),
		Hint:    "run: barista gradle list; fix with: barista gradle set-default <name>",
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
