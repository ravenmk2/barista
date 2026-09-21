package output

import (
	"strings"
	"testing"
)

func TestPaletteTrueColorDark(t *testing.T) {
	p := newPalette(true, true, "truecolor")
	cases := map[string]struct {
		got  string
		want string
	}{
		"green":   {p.Green("x"), "\x1b[38;2;18;199;143mx\x1b[0m"},
		"red":     {p.Red("x"), "\x1b[38;2;255;110;99mx\x1b[0m"},
		"yellow":  {p.Yellow("x"), "\x1b[38;2;245;239;52mx\x1b[0m"},
		"cyan":    {p.Cyan("x"), "\x1b[38;2;10;220;217mx\x1b[0m"},
		"blue":    {p.Blue("x"), "\x1b[38;2;114;114;255mx\x1b[0m"},
		"magenta": {p.Magenta("x"), "\x1b[38;2;255;96;255mx\x1b[0m"},
		"dim":     {p.Dim("x"), "\x1b[2mx\x1b[0m"},
		"ybold":   {p.YellowBold("x"), "\x1b[1;38;2;245;239;52mx\x1b[0m"},
	}
	for name, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", name, tc.got, tc.want)
		}
	}
}

func TestPaletteTrueColorLight(t *testing.T) {
	p := newPalette(true, false, "truecolor")
	cases := map[string]struct {
		got  string
		want string
	}{
		"green":   {p.Green("x"), "\x1b[38;2;12;179;127mx\x1b[0m"},
		"red":     {p.Red("x"), "\x1b[38;2;235;66;104mx\x1b[0m"},
		"yellow":  {p.Yellow("x"), "\x1b[38;2;156;156;0mx\x1b[0m"},
		"cyan":    {p.Cyan("x"), "\x1b[38;2;16;177;174mx\x1b[0m"},
		"blue":    {p.Blue("x"), "\x1b[38;2;0;164;255mx\x1b[0m"},
		"magenta": {p.Magenta("x"), "\x1b[38;2;195;55;224mx\x1b[0m"},
		"ybold":   {p.YellowBold("x"), "\x1b[1;38;2;156;156;0mx\x1b[0m"},
	}
	for name, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", name, tc.got, tc.want)
		}
	}
}

func TestPaletteProfile256(t *testing.T) {
	p := newPalette(true, true, "256")
	if got := p.Green("x"); got != "\x1b[38;5;42mx\x1b[0m" {
		t.Errorf("got %q", got)
	}
	if got := p.Blue("x"); got != "\x1b[38;5;63mx\x1b[0m" {
		t.Errorf("got %q", got)
	}
}

func TestPaletteProfile16Distinct(t *testing.T) {
	for _, dark := range []bool{true, false} {
		p := newPalette(true, dark, "16")
		roles := []func(string) string{p.Green, p.Red, p.Yellow, p.Cyan, p.Blue, p.Magenta}
		seen := map[string]bool{}
		for _, role := range roles {
			got := role("x")
			if seen[got] {
				t.Errorf("dark=%v: roles collide in 16-color fallback: %q", dark, got)
			}
			seen[got] = true
		}
	}
}

func TestPaletteProfile16DarkIsBright(t *testing.T) {
	p := newPalette(true, true, "16")
	roles := []func(string) string{p.Green, p.Red, p.Yellow, p.Cyan, p.Blue, p.Magenta}
	for _, role := range roles {
		if got := role("x"); !strings.HasPrefix(got, "\x1b[9") {
			t.Errorf("dark 16-color profile should emit bright SGR codes, got %q", got)
		}
	}
}

func TestPaletteDisabledPassthrough(t *testing.T) {
	p := NewPalette(false, "truecolor")
	for _, role := range []func(string) string{p.Green, p.Red, p.Yellow, p.Cyan, p.Blue, p.Magenta, p.Dim, p.YellowBold, p.Branch} {
		if got := role("plain"); got != "plain" {
			t.Errorf("disabled palette should pass through, got %q", got)
		}
	}
}

func TestPaletteEmptyString(t *testing.T) {
	p := NewPalette(true, "truecolor")
	if got := p.Green(""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
}

func TestPaletteUnknownProfileFallsBackToAuto(t *testing.T) {
	if got, want := NewPalette(true, "bogus").Green("x"), NewPalette(true, "auto").Green("x"); got != want {
		t.Errorf("unknown profile should behave like auto: got %q, want %q", got, want)
	}
}
