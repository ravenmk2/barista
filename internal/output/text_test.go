package output

import (
	"maps"
	"testing"
)

func TestBranchClass(t *testing.T) {
	cases := map[string]branchClassT{
		"main":             classTrunk,
		"master":           classTrunk,
		"trunk":            classTrunk,
		"develop":          classDev,
		"dev":              classDev,
		"feature/login":    classFeature,
		"release/1.2":      classRelease,
		"hotfix/urgent":    classFix,
		"fix/typo":         classFix,
		"bugfix/crash":     classFix,
		"origin/main":      classTrunk,
		"origin/feature/x": classFeature,
		"custom/x":         classOther,
		"feature":          classOther,
		"features/x":       classOther,
		"fix":              classOther,
		"maintenance":      classOther,
		"":                 classOther,
	}
	for branch, want := range cases {
		if got := branchClass(branch); got != want {
			t.Errorf("branchClass(%q) = %d, want %d", branch, got, want)
		}
	}
}

func TestBranchPaletteDisabled(t *testing.T) {
	p := NewPalette(false)
	for _, b := range []string{"main", "develop", "feature/x", "release/1.0", "hotfix/y", "custom/z"} {
		if got := p.Branch(b); got != b {
			t.Errorf("disabled palette Branch(%q) = %q, want unchanged", b, got)
		}
	}
}

func TestBranchPaletteEnabled(t *testing.T) {
	p := NewPalette(true)
	if got := p.Branch("main"); got != "\x1b[32mmain\x1b[0m" {
		t.Errorf("trunk branch should be green, got %q", got)
	}
	if got := p.Branch("custom/x"); got != "custom/x" {
		t.Errorf("unclassified branch should stay uncolored, got %q", got)
	}
}

func TestNameWidth(t *testing.T) {
	if got := NameWidth(nil); got != 1 {
		t.Errorf("empty names: got %d, want 1", got)
	}
	if got := NameWidth([]string{"a", "order-service", "api"}); got != len("order-service") {
		t.Errorf("got %d, want %d", got, len("order-service"))
	}
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'x'
	}
	if got := NameWidth([]string{string(long)}); got != maxNameWidth {
		t.Errorf("overlong name: got %d, want cap %d", got, maxNameWidth)
	}
}

func TestPadName(t *testing.T) {
	if got := padName("api", 5); got != "api  " {
		t.Errorf("pad: got %q", got)
	}
	long := "order-service-with-a-very-long-name"
	if got := padName(long, 10); got != "order-ser…" {
		t.Errorf("truncate: got %q", got)
	}
	if got := padName("服务abc", 5); len([]rune(got)) != 5 {
		t.Errorf("multibyte pad: got %q (runes %d)", got, len([]rune(got)))
	}
}
func TestStatusDetailTrackedMarker(t *testing.T) {
	p := NewPalette(false)
	base := map[string]any{"ahead": 1, "behind": 2, "staged": 0, "modified": 3, "untracked": 0}

	tracked := Result{Branch: "main", Detail: map[string]any{"tracked": true}}
	maps.Copy(tracked.Detail, base)
	if got, want := statusDetail(p, tracked), "main ahead=1 behind=2 staged=0 modified=3 untracked=0"; got != want {
		t.Errorf("tracked: got %q, want %q", got, want)
	}

	local := Result{Branch: "feature/x", Detail: map[string]any{"tracked": false}}
	maps.Copy(local.Detail, base)
	if got, want := statusDetail(p, local), "feature/x* ahead=- behind=- staged=0 modified=3 untracked=0"; got != want {
		t.Errorf("untracked: got %q, want %q", got, want)
	}
}
