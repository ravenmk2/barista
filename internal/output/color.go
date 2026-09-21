package output

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
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
// and degrade to the palette's color profile (truecolor -> 256 -> 16). Dark
// and light themes adapt to the terminal background; detection mirrors fang
// so command output and --help always render the same hues.
type Palette struct {
	enabled bool
	profile colorprofile.Profile
	colors  paletteColors
}

type paletteColors struct {
	green   color.Color
	red     color.Color
	yellow  color.Color
	cyan    color.Color
	blue    color.Color
	magenta color.Color
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
	return newPalette(true, detectDark(), name)
}

func newPalette(enabled bool, dark bool, profileName string) Palette {
	if !enabled {
		return Palette{}
	}
	colors := lightColors
	if dark {
		colors = darkColors
	}
	return Palette{enabled: true, profile: resolveProfile(profileName), colors: colors}
}

// detectDark uses the same heuristic as fang's help theme: query the
// terminal background when stdout is a real terminal, assume light otherwise
// (no query possible, e.g. Git Bash pty or color: always into a pipe).
func detectDark() bool {
	return term.IsTerminal(os.Stdout.Fd()) && lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
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

// Semantic colors in the charmtone family, the same palette fang's
// DefaultColorScheme draws --help from. Both themes land the six roles on
// distinct 16-color fallbacks (dark: bright 10/9/11/14/12/13; light: darker
// greens/cyans read better on light backgrounds).
var (
	darkColors = paletteColors{
		green:   rgb(0x12C78F), // charmtone.Guac (fang Flag)
		red:     rgb(0xFF6E63), // charmtone.Bengal
		yellow:  rgb(0xF5EF34), // charmtone.Mustard
		cyan:    rgb(0x0ADCD9), // charmtone.Turtle
		blue:    rgb(0x7272FF), // charmtone.Guppy (fang Program)
		magenta: rgb(0xFF60FF), // charmtone.Dolly
	}
	lightColors = paletteColors{
		green:   rgb(0x0CB37F), // fang light Flag
		red:     rgb(0xEB4268), // charmtone.Sriracha
		yellow:  rgb(0x9C9C00), // amber, lands on non-bright yellow in 16 colors
		cyan:    rgb(0x10B1AE), // charmtone.Zinc
		blue:    rgb(0x00A4FF), // charmtone.Malibu (fang light Program)
		magenta: rgb(0xC337E0), // charmtone.Urchin
	}
)

func rgb(h uint32) color.RGBA {
	return color.RGBA{R: uint8(h >> 16), G: uint8(h >> 8), B: uint8(h), A: 0xFF}
}

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

func (p Palette) Green(s string) string   { return p.paint(p.colors.green, s) }
func (p Palette) Red(s string) string     { return p.paint(p.colors.red, s) }
func (p Palette) Yellow(s string) string  { return p.paint(p.colors.yellow, s) }
func (p Palette) Cyan(s string) string    { return p.paint(p.colors.cyan, s) }
func (p Palette) Blue(s string) string    { return p.paint(p.colors.blue, s) }
func (p Palette) Magenta(s string) string { return p.paint(p.colors.magenta, s) }

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
	cc := p.profile.Convert(p.colors.yellow)
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
