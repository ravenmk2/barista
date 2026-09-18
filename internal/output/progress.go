package output

import (
	"fmt"
	"strings"
	"time"
)

func ProgressLine(received, total int64, elapsed time.Duration) string {
	const width = 25
	rate := float64(received) / max(elapsed.Seconds(), 0.5)
	speed := HumanBytes(int64(rate)) + "/s"
	if total <= 0 {
		return fmt.Sprintf("downloading %s (%s)", HumanBytes(received), speed)
	}
	pct := float64(received) / float64(total)
	bar := "[" + strings.Repeat("=", int(pct*width)) + strings.Repeat("-", width-int(pct*width)) + "]"
	line := fmt.Sprintf("downloading %s %5.1f%% %s/%s at %s", bar, pct*100, HumanBytes(received), HumanBytes(total), speed)
	if rate > 0 {
		eta := time.Duration(float64(total-received) / rate * float64(time.Second))
		line += fmt.Sprintf(", eta %s", eta.Round(time.Second))
	}
	return line
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
