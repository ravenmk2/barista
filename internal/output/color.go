package output

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

type styler interface {
	Green(string) string
	Red(string) string
	Yellow(string) string
	Cyan(string) string
	Blue(string) string
	Magenta(string) string
	Dim(string) string
	YellowBold(string) string
	Branch(string) string
}

// Palette styles human-facing text output. Colors are authored in 24-bit hex
// and degrade to the palette's color profile (truecolor -> 256 -> 16).
type Palette struct {
	enabled bool
	profile colorprofile.Profile
}

// NewPalette builds a Palette. profileName is "", "auto", "truecolor", "256",
// or "16"; empty means auto. When enabled is false every method passes its
// input through unchanged.
func NewPalette(enabled bool, profileName ...string) Palette {
	if !enabled {
		return Palette{}
	}
	name := "auto"
	if len(profileName) > 0 && profileName[0] != "" {
		name = profileName[0]
	}
	return Palette{enabled: true, profile: resolveProfile(name)}
}

// Enabled reports whether styling is active.
func (p Palette) Enabled() bool { return p.enabled }

func resolveProfile(name string) colorprofile.Profile {
	switch name {
	case "truecolor":
		return colorprofile.TrueColor
	case "256":
		return colorprofile.ANSI256
	case "16":
		return colorprofile.ANSI
	}
	p := colorprofile.Detect(os.Stdout, os.Environ())
	if p <= colorprofile.Ascii {
		// Whether to color was already decided by ColorEnabled; honor that
		// decision (e.g. color: always into a pipe) instead of the detection.
		return colorprofile.TrueColor
	}
	return p
}

// Semantic colors. Picked so that the 16-color fallback lands on six distinct
// basic colors (10/9/11/14/12/13), keeping roles distinguishable everywhere.
var (
	colGreen   = color.RGBA{R: 0x2E, G: 0xCC, B: 0x71, A: 0xFF}
	colRed     = color.RGBA{R: 0xFF, G: 0x55, B: 0x55, A: 0xFF}
	colYellow  = color.RGBA{R: 0xF5, G: 0xEF, B: 0x34, A: 0xFF}
	colCyan    = color.RGBA{R: 0x0A, G: 0xDC, B: 0xD9, A: 0xFF}
	colBlue    = color.RGBA{R: 0x72, G: 0x72, B: 0xFF, A: 0xFF}
	colMagenta = color.RGBA{R: 0xFF, G: 0x60, B: 0xFF, A: 0xFF}
)

// fgSeq renders c as SGR foreground parameters.
func fgSeq(c color.Color) string {
	switch c := c.(type) {
	case ansi.BasicColor:
		if c < 8 {
			return fmt.Sprintf("3%d", c)
		}
		return fmt.Sprintf("9%d", c-8)
	case ansi.IndexedColor:
		return fmt.Sprintf("38;5;%d", c)
	default:
		r, g, b, _ := c.RGBA()
		return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
	}
}

func (p Palette) paint(c color.Color, s string) string {
	if !p.enabled || s == "" {
		return s
	}
	cc := p.profile.Convert(c)
	if cc == nil {
		return s
	}
	return "\x1b[" + fgSeq(cc) + "m" + s + "\x1b[0m"
}

func (p Palette) Green(s string) string   { return p.paint(colGreen, s) }
func (p Palette) Red(s string) string     { return p.paint(colRed, s) }
func (p Palette) Yellow(s string) string  { return p.paint(colYellow, s) }
func (p Palette) Cyan(s string) string    { return p.paint(colCyan, s) }
func (p Palette) Blue(s string) string    { return p.paint(colBlue, s) }
func (p Palette) Magenta(s string) string { return p.paint(colMagenta, s) }

func (p Palette) Dim(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

func (p Palette) YellowBold(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	cc := p.profile.Convert(colYellow)
	if cc == nil {
		return s
	}
	return "\x1b[1;" + fgSeq(cc) + "m" + s + "\x1b[0m"
}

func (p Palette) Branch(s string) string {
	switch branchClass(s) {
	case classTrunk:
		return p.Green(s)
	case classDev:
		return p.Blue(s)
	case classFeature:
		return p.Yellow(s)
	case classRelease:
		return p.Cyan(s)
	case classFix:
		return p.Magenta(s)
	}
	return s
}

type branchClassT int

const (
	classOther branchClassT = iota
	classTrunk
	classDev
	classFeature
	classRelease
	classFix
)

func branchClass(branch string) branchClassT {
	b := strings.TrimPrefix(branch, "origin/")
	switch b {
	case "main", "master", "trunk":
		return classTrunk
	case "develop", "dev":
		return classDev
	}
	switch {
	case strings.HasPrefix(b, "feature/"):
		return classFeature
	case strings.HasPrefix(b, "release/"):
		return classRelease
	case strings.HasPrefix(b, "hotfix/"), strings.HasPrefix(b, "fix/"), strings.HasPrefix(b, "bugfix/"):
		return classFix
	}
	return classOther
}

func ColorEnabled(mode string) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	switch mode {
	case "never":
		return false
	case "always":
		return true
	}
	return StdoutIsTerminal()
}

func StdoutIsTerminal() bool {
	return stdoutIsTerminal(os.Stdout)
}

func StderrIsTerminal() bool {
	return stdoutIsTerminal(os.Stderr)
}

func StdinIsTerminal() bool {
	return stdoutIsTerminal(os.Stdin)
}
