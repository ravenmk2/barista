package output

import "os"

type styler interface {
	Green(string) string
	Red(string) string
	Yellow(string) string
	Cyan(string) string
	Dim(string) string
	YellowBold(string) string
}

type Palette struct {
	enabled bool
}

func NewPalette(enabled bool) Palette { return Palette{enabled: enabled} }

func (p Palette) wrap(code, s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p Palette) Green(s string) string      { return p.wrap("32", s) }
func (p Palette) Red(s string) string        { return p.wrap("31", s) }
func (p Palette) Yellow(s string) string     { return p.wrap("33", s) }
func (p Palette) Cyan(s string) string       { return p.wrap("36", s) }
func (p Palette) Dim(s string) string        { return p.wrap("2", s) }
func (p Palette) YellowBold(s string) string { return p.wrap("1;33", s) }

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
