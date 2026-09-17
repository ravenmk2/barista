package gitrun

import (
	"context"
	"strings"

	"barista/internal/workspace"
)

type DefaultBranchError struct{}

func (e *DefaultBranchError) Error() string {
	return "cannot resolve default branch"
}

type defaultBranchCandidate struct {
	branch string
	source string
}

func ResolveDefaultBranch(ctx context.Context, dir string, repo workspace.Repo, topDefault string) (branch, source, base string, err error) {
	var candidates []defaultBranchCandidate
	if repo.DefaultBranch != "" {
		candidates = append(candidates, defaultBranchCandidate{repo.DefaultBranch, "repo"})
	}
	if topDefault != "" {
		candidates = append(candidates, defaultBranchCandidate{topDefault, "top"})
	}
	out, symErr := run(ctx, dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if symErr == nil {
		if b, ok := strings.CutPrefix(strings.TrimSpace(out), "refs/remotes/origin/"); ok && b != "" {
			candidates = append(candidates, defaultBranchCandidate{b, "origin-head"})
		}
	}
	for _, b := range []string{"main", "master", "trunk"} {
		candidates = append(candidates, defaultBranchCandidate{b, "probe"})
	}
	for _, c := range candidates {
		if refExists(ctx, dir, "refs/remotes/origin/"+c.branch) {
			return c.branch, c.source, "origin/" + c.branch, nil
		}
		if refExists(ctx, dir, "refs/heads/"+c.branch) {
			return c.branch, c.source, c.branch, nil
		}
	}
	return "", "", "", &DefaultBranchError{}
}
