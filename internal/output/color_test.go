package output

import (
	"strings"
	"testing"
)

func TestPaletteTrueColor(t *testing.T) {
	p := NewPalette(true, "truecolor")
	cases := map[string]struct {
		got  string
		want string
	}{
		"green":   {p.Green("x"), "\x1b[38;2;46;204;113mx\x1b[0m"},
		"red":     {p.Red("x"), "\x1b[38;2;255;85;85mx\x1b[0m"},
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

func TestPaletteProfile256(t *testing.T) {
	p := NewPalette(true, "256")
	if got := p.Green("x"); got != "\x1b[38;5;41mx\x1b[0m" {
		t.Errorf("got %q", got)
	}
	if got := p.Blue("x"); got != "\x1b[38;5;63mx\x1b[0m" {
		t.Errorf("got %q", got)
	}
}

func TestPaletteProfile16Distinct(t *testing.T) {
	p := NewPalette(true, "16")
	roles := []func(string) string{p.Green, p.Red, p.Yellow, p.Cyan, p.Blue, p.Magenta}
	seen := map[string]bool{}
	for _, role := range roles {
		got := role("x")
		if !strings.HasPrefix(got, "\x1b[9") {
			t.Errorf("16-color profile should emit bright SGR codes, got %q", got)
		}
		if seen[got] {
			t.Errorf("roles collide in 16-color fallback: %q", got)
		}
		seen[got] = true
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
