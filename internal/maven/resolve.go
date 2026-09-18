package maven

import (
	"fmt"

	"barista/internal/output"
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
		segs, _, _ := ParseVersion(arg)
		for depth := len(segs); depth >= 1; depth-- {
			var best *Entry
			for i := range r.Installations {
				e := &r.Installations[i]
				es, _, err := ParseVersion(e.Version)
				if err != nil || len(es) < depth {
					continue
				}
				if !equalSegs(es[:depth], segs[:depth]) {
					continue
				}
				if best == nil || CompareVersions(e.Version, best.Version) > 0 {
					best = e
				}
			}
			if best != nil {
				return best, "prefix", nil
			}
		}
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeMavenNotFound,
		Message: fmt.Sprintf("no registered maven matches %q", arg),
		Hint:    "run: barista maven list",
	}
}

func (r *Registry) ResolveDefault() (*Entry, string, *output.ErrInfo) {
	if r.Default == "" {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeMavenNotFound,
			Message: "no default maven configured",
			Hint:    "set one with: barista maven set-default <name>",
		}
	}
	if e := r.Find(r.Default); e != nil {
		return e, "default", nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeMavenNotFound,
		Message: fmt.Sprintf("default maven %q is not registered", r.Default),
		Hint:    "run: barista maven list; fix with: barista maven set-default <name>",
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
