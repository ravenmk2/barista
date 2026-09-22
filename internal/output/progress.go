package output

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"
)

const progressBarMaxWidth = 40

var partialBlocks = []rune(" ▏▎▍▌▋▊▉")

// ProgressLine renders one \r-rewritable progress line, padded with trailing
// spaces to the stderr width so the previous render is fully overwritten.
func ProgressLine(p Palette, received, total int64, elapsed time.Duration) string {
	return progressLine(p, received, total, elapsed, stderrWidth())
}

func stderrWidth() int {
	if w, _, err := term.GetSize(os.Stderr.Fd()); err == nil && w > 0 {
		return w
	}
	return 80
}

func progressLine(p Palette, received, total int64, elapsed time.Duration, width int) string {
	rate := float64(received) / max(elapsed.Seconds(), 0.5)
	speed := HumanBytes(int64(rate)) + "/s"
	plain, colored := "", ""
	if total <= 0 {
		s := fmt.Sprintf("downloading  %s  %s", HumanBytes(received), speed)
		plain, colored = s, s
	} else {
		pct := min(max(float64(received)/float64(total), 0), 1)
		rest := fmt.Sprintf("  %5.1f%%  %s/%s  %s", pct*100, HumanBytes(received), HumanBytes(total), speed)
		if rate > 0 {
			eta := time.Duration(float64(total-received) / rate * float64(time.Second)).Round(time.Second)
			rest += "  eta " + eta.String()
		}
		barWidth := min(max(width-len("downloading  ")-len(rest)-1, 10), progressBarMaxWidth)
		barPlain, barColored := progressBar(p, pct, barWidth)
		plain = "downloading  " + barPlain + rest
		colored = "downloading  " + barColored + rest
	}
	if pad := width - 1 - utf8.RuneCountInString(plain); pad > 0 {
		colored += strings.Repeat(" ", pad)
	}
	return colored
}

func progressBar(p Palette, pct float64, width int) (plain, colored string) {
	pct = min(max(pct, 0), 1)
	exact := pct * float64(width)
	full := int(exact)
	var b strings.Builder
	b.WriteString(strings.Repeat("█", full))
	used := full
	if used < width {
		if idx := int((exact - float64(full)) * 8); idx > 0 {
			b.WriteRune(partialBlocks[idx])
			used++
		}
	}
	filled := b.String()
	empty := strings.Repeat("░", width-used)
	plain = filled + empty
	if p.Enabled() {
		return plain, p.Cyan(filled) + p.Dim(empty)
	}
	return plain, plain
}

func HumanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/float64(int64(1)<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/float64(int64(1)<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/float64(int64(1)<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
