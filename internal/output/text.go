package output

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type TextRenderer struct {
	w         io.Writer
	command   string
	p         Palette
	nameWidth int
}

const maxNameWidth = 40

func NameWidth(names []string) int {
	w := 1
	for _, n := range names {
		w = max(w, utf8.RuneCountInString(n))
	}
	return min(w, maxNameWidth)
}

func padName(s string, w int) string {
	if n := utf8.RuneCountInString(s); n > w {
		return string([]rune(s)[:w-1]) + "…"
	} else {
		return s + strings.Repeat(" ", w-n)
	}
}

func NewTextRenderer(w io.Writer, command string, color bool, nameWidth int) *TextRenderer {
	return &TextRenderer{w: w, command: command, p: NewPalette(color), nameWidth: max(nameWidth, 1)}
}

func (r *TextRenderer) OnResult(_ int, res Result) {
	status := fmt.Sprintf("%-8s", res.Status)
	name := padName(res.Name, r.nameWidth)
	if res.Status != StatusOK {
		line := status + " " + name + " " + describe(Palette{}, res)
		if res.Status == StatusSkipped {
			_, _ = fmt.Fprintln(r.w, r.p.Dim(line))
		} else {
			_, _ = fmt.Fprintln(r.w, r.p.Red(line))
		}
		return
	}
	if r.attention(res) {
		name = r.p.YellowBold(name)
	}
	_, _ = fmt.Fprintf(r.w, "%s %s %s\n", r.p.Green(status), name, describe(r.p, res))
}

func (r *TextRenderer) attention(res Result) bool {
	return r.command == "git status" && res.Changed()
}

func (r *TextRenderer) Finish(results []Result) {
	s := Summarize(results)
	p := r.p
	seg := func(v int, label string, style func(string) string) string {
		t := fmt.Sprintf("%d %s", v, label)
		if v > 0 {
			return style(t)
		}
		return p.Dim(t)
	}
	var line string
	if s.Total > 0 && s.OK == s.Total {
		line = p.Green(fmt.Sprintf("%d repos: %d ok", s.Total, s.OK)) +
			p.Dim(fmt.Sprintf(", %d skipped, %d failed", s.Skipped, s.Failed))
	} else {
		line = fmt.Sprintf("%d repos: ", s.Total) +
			seg(s.OK, "ok", p.Green) + ", " +
			seg(s.Skipped, "skipped", p.Yellow) + ", " +
			seg(s.Failed, "failed", p.Red)
	}
	_, _ = fmt.Fprintln(r.w, "\n"+line)
	switch r.command {
	case "git status":
		r.trailer("Repositories with changes: ", selectNames(results, func(res Result) bool { return res.Changed() }))
	case "git checkout":
		r.trailer("Dirty worktrees (skipped): ", selectNames(results, func(res Result) bool {
			return res.Error != nil && res.Error.Code == CodeDirtyWorktree
		}))
	}
}

func selectNames(results []Result, match func(Result) bool) []string {
	var names []string
	for _, res := range results {
		if match(res) {
			names = append(names, res.Name)
		}
	}
	return names
}

func (r *TextRenderer) trailer(title string, names []string) {
	if len(names) == 0 {
		return
	}
	styled := make([]string, len(names))
	for i, n := range names {
		styled[i] = r.p.YellowBold(n)
	}
	_, _ = fmt.Fprintln(r.w, title+strings.Join(styled, ", "))
}

func describe(p styler, res Result) string {
	if res.Error != nil {
		return fmt.Sprintf("%s: %s", res.Error.Code, res.Error.Message)
	}
	d := res.Detail
	switch res.Action {
	case "clone":
		return fmt.Sprintf("cloned from %v (branch %s)", d["url"], p.Branch(fmt.Sprint(d["branch"])))
	case "status":
		return statusDetail(p, res)
	case "checkout":
		switch d["action"] {
		case "created":
			return fmt.Sprintf("created from %s (default branch source: %v)",
				p.Branch(fmt.Sprint(d["baseBranch"])), d["defaultBranchSource"])
		case "created-tracking":
			return "created tracking " + p.Branch("origin/"+res.Branch)
		default:
			return "switched to " + p.Branch(res.Branch)
		}
	case "fetch":
		return fmt.Sprintf("%s fetched (prune=%v)", p.Branch(res.Branch), d["prune"])
	case "pull":
		return fmt.Sprintf("%s pulled (rebase=%v)", p.Branch(res.Branch), d["rebase"])
	case "push":
		verb := "pushed"
		if d["tags"] == true {
			verb = "pushed tags"
		} else if d["setUpstream"] == true {
			verb = "pushed and set upstream"
		}
		return p.Branch(res.Branch) + " " + verb
	}
	return "ok"
}

func statusDetail(p styler, res Result) string {
	get := func(k string) int {
		n, _ := res.Detail[k].(int)
		return n
	}
	num := func(label string, v int, style func(string) string) string {
		s := fmt.Sprintf("%s=%d", label, v)
		if v > 0 {
			return style(s)
		}
		return p.Dim(s)
	}
	tracked, _ := res.Detail["tracked"].(bool)
	branch := p.Branch(res.Branch)
	ab := func(label string, v int) string { return num(label, v, p.Yellow) }
	if !tracked {
		branch += "*"
		ab = func(label string, _ int) string { return p.Dim(label + "=-") }
	}
	return strings.Join([]string{
		branch,
		ab("ahead", get("ahead")),
		ab("behind", get("behind")),
		num("staged", get("staged"), p.Green),
		num("modified", get("modified"), p.Red),
		num("untracked", get("untracked"), p.Red),
	}, " ")
}
