package output

import (
	"strings"
	"testing"
	"time"
)

var plain = NewPalette(false)

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		500:              "500B",
		2048:             "2.0K",
		5 * 1024 * 1024:  "5.0M",
		2 << 30:          "2.0G",
		1536 * 1024 * 10: "15.0M",
	}
	for in, want := range cases {
		if got := HumanBytes(in); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestProgressLine(t *testing.T) {
	line := progressLine(plain, 50<<20, 100<<20, 10*time.Second, 100)
	for _, want := range []string{"50.0%", "50.0M/100.0M", "5.0M/s", "eta 10s", "█", "░"} {
		if !strings.Contains(line, want) {
			t.Errorf("progressLine missing %q in %q", want, line)
		}
	}
	if strings.Count(line, "█")+strings.Count(line, "░")+strings.Count(line, "▌") < 10 {
		t.Errorf("progress bar too short in %q", line)
	}
}

func TestProgressLinePartialBlock(t *testing.T) {
	bar, _ := progressBar(plain, 0.55, 10)
	if !strings.ContainsAny(bar, "▏▎▍▌▋▊▉") {
		t.Errorf("want a partial block in %q", bar)
	}
	bar, _ = progressBar(plain, 1.5, 10)
	if bar != strings.Repeat("█", 10) {
		t.Errorf("over-100%% bar = %q, want full bar", bar)
	}
}

func TestProgressLineColored(t *testing.T) {
	p := NewPalette(true, "truecolor")
	_, colored := progressBar(p, 0.5, 10)
	if !strings.Contains(colored, "\x1b[") {
		t.Errorf("want ANSI styling in colored bar, got %q", colored)
	}
}

func TestProgressLinePadsToWidth(t *testing.T) {
	line := progressLine(plain, 50<<20, 100<<20, 10*time.Second, 100)
	if !strings.HasSuffix(line, " ") {
		t.Errorf("want trailing padding to overwrite previous render: %q", line)
	}
}

func TestProgressLineUnknownTotal(t *testing.T) {
	line := progressLine(plain, 3<<20, -1, 2*time.Second, 100)
	if !strings.Contains(line, "3.0M") || strings.Contains(line, "%") || strings.Contains(line, "eta") {
		t.Errorf("unexpected line for unknown total: %q", line)
	}
}
