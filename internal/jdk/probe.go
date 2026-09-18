package jdk

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"barista/internal/output"
)

type Info struct {
	Home    string
	Version string
	Major   int
	Distro  string
}

const probeTimeout = 15 * time.Second

func Probe(path string) (Info, *output.ErrInfo) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Info{}, &output.ErrInfo{Code: output.CodeNotAJDK, Message: err.Error()}
	}
	home, ok := resolveHome(abs)
	if !ok {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNotAJDK,
			Message: fmt.Sprintf("%s: no bin/%s found (not a JDK home)", filepath.ToSlash(abs), exeName("java")),
		}
	}
	if _, err := os.Stat(filepath.Join(home, "bin", exeName("javac"))); err != nil {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeNotAJDK,
			Message: fmt.Sprintf("%s: missing bin/%s (a JRE is not a JDK)", filepath.ToSlash(home), exeName("javac")),
		}
	}
	text, err := runJava(home, "-XshowSettings:properties", "-version")
	version := ""
	if err == nil {
		version = propsValue(text, "java.version")
	}
	if version == "" {
		fallback, ferr := runJava(home, "-version")
		if ferr != nil && err != nil {
			return Info{}, &output.ErrInfo{
				Code:    output.CodeJDKProbeFailed,
				Message: fmt.Sprintf("%s: cannot run java: %v", filepath.ToSlash(home), ferr),
			}
		}
		version = parseVersionLine(fallback)
		text += fallback
	}
	if version == "" {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeJDKProbeFailed,
			Message: fmt.Sprintf("%s: cannot parse java version from output", filepath.ToSlash(home)),
		}
	}
	major, _, perr := ParseVersion(version)
	if perr != nil {
		return Info{}, &output.ErrInfo{
			Code:    output.CodeJDKProbeFailed,
			Message: fmt.Sprintf("%s: cannot parse java version %q", filepath.ToSlash(home), version),
		}
	}
	return Info{Home: home, Version: version, Major: major, Distro: detectDistro(distroEvidence(text))}, nil
}

func distroEvidence(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if k, _, ok := strings.Cut(trimmed, " = "); ok {
				switch k {
				case "java.vendor", "java.vm.vendor", "java.vm.name":
					b.WriteString(trimmed + "\n")
				}
			}
			continue
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func resolveHome(path string) (string, bool) {
	if hasJava(path) {
		return path, true
	}
	nested := filepath.Join(path, "Contents", "Home")
	if hasJava(nested) {
		return nested, true
	}
	return "", false
}

func hasJava(home string) bool {
	fi, err := os.Stat(filepath.Join(home, "bin", exeName("java")))
	return err == nil && !fi.IsDir()
}

func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func runJava(home string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(home, "bin", exeName("java")), args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("java timed out after %s", probeTimeout)
	}
	return string(out), err
}

func propsValue(text, key string) string {
	prefix := key + " = "
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

var versionLineRe = regexp.MustCompile(`(?i)\b(?:openjdk|java) version "([^"]+)"`)
var quotedRe = regexp.MustCompile(`"([^"]+)"`)

func parseVersionLine(text string) string {
	if m := versionLineRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	if m := quotedRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

func detectDistro(text string) string {
	l := strings.ToLower(text)
	switch {
	case strings.Contains(l, "temurin"), strings.Contains(l, "adoptium"):
		return "temurin"
	case strings.Contains(l, "corretto"):
		return "corretto"
	case strings.Contains(l, "zulu"):
		return "zulu"
	case strings.Contains(l, "microsoft"):
		return "microsoft"
	case strings.Contains(l, "semeru"):
		return "semeru"
	case strings.Contains(l, "graalvm"):
		return "graalvm"
	case strings.Contains(l, "liberica"):
		return "liberica"
	case strings.Contains(l, "sapmachine"):
		return "sapmachine"
	case strings.Contains(l, "java(tm)"), strings.Contains(l, "hotspot(tm)"):
		return "oracle"
	case strings.Contains(l, "openjdk"):
		return "openjdk"
	default:
		return "temurin"
	}
}
