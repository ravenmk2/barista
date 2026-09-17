package gitrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = "..." + msg[len(msg)-500:]
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}

func refExists(ctx context.Context, dir, ref string) bool {
	_, err := run(ctx, dir, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func isGitRepo(ctx context.Context, dir string) bool {
	out, err := run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func currentBranch(ctx context.Context, dir string) string {
	out, err := run(ctx, dir, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
