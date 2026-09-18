package output

import (
	"strings"
	"testing"
	"time"
)

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
	line := ProgressLine(50<<20, 100<<20, 10*time.Second)
	for _, want := range []string{"[============-------------]", "50.0%", "50.0M/100.0M", "5.0M/s", "eta 10s"} {
		if !strings.Contains(line, want) {
			t.Errorf("ProgressLine missing %q in %q", want, line)
		}
	}
}

func TestProgressLineUnknownTotal(t *testing.T) {
	line := ProgressLine(3<<20, -1, 2*time.Second)
	if !strings.Contains(line, "3.0M") || strings.Contains(line, "%") || strings.Contains(line, "eta") {
		t.Errorf("unexpected line for unknown total: %q", line)
	}
}
