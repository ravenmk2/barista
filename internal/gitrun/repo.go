package gitrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"barista/internal/output"
	"barista/internal/workspace"
)

type RepoStatus struct {
	Branch      string
	Ahead       int
	Behind      int
	Staged      int
	Modified    int
	Untracked   int
	HasUpstream bool
}

func parseStatus(ctx context.Context, dir string) (RepoStatus, error) {
	out, err := run(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return RepoStatus{}, err
	}
	var s RepoStatus
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			s.Branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
		case strings.HasPrefix(line, "# branch.upstream "):
			s.HasUpstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &s.Ahead, &s.Behind)
		case strings.HasPrefix(line, "? "):
			s.Untracked++
		case len(line) >= 4 && (line[0] == '1' || line[0] == '2' || line[0] == 'u') && line[1] == ' ':
			xy := line[2:4]
			if xy[0] != '.' && xy[0] != ' ' {
				s.Staged++
			}
			if xy[1] != '.' && xy[1] != ' ' {
				s.Modified++
			}
		}
	}
	return s, nil
}

func newResult(repo workspace.Repo, action string) output.Result {
	return output.Result{
		Name:   repo.Name,
		Path:   repo.Path,
		Action: action,
	}
}

func finish(res *output.Result, start time.Time) {
	res.DurationMs = time.Since(start).Milliseconds()
}

func fail(res *output.Result, code string, err error) output.Result {
	res.Status = output.StatusFailed
	res.Error = &output.ErrInfo{Code: code, Message: err.Error()}
	return *res
}

func skip(res *output.Result, code, message string) output.Result {
	res.Status = output.StatusSkipped
	res.Error = &output.ErrInfo{Code: code, Message: message}
	return *res
}

func Clone(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
	start := time.Now()
	res := newResult(repo, "clone")
	defer finish(&res, start)
	dir := ws.AbsPath(repo)
	if isGitRepo(ctx, dir) {
		res.Branch = currentBranch(ctx, dir)
		return skip(&res, output.CodeRepoExists, "repository already exists")
	}
	if _, err := run(ctx, ws.Root, "clone", repo.ResolvedURL, repo.Path); err != nil {
		return fail(&res, output.CodeGitError, err)
	}
	res.Status = output.StatusOK
	res.Branch = currentBranch(ctx, dir)
	res.Detail = map[string]any{"url": repo.ResolvedURL, "branch": res.Branch}
	return res
}

func Status(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
	start := time.Now()
	res := newResult(repo, "status")
	defer finish(&res, start)
	dir := ws.AbsPath(repo)
	if !isGitRepo(ctx, dir) {
		return skip(&res, output.CodeNotCloned, "repository not cloned")
	}
	st, err := parseStatus(ctx, dir)
	if err != nil {
		return fail(&res, output.CodeGitError, err)
	}
	res.Status = output.StatusOK
	res.Branch = st.Branch
	res.Detail = map[string]any{
		"ahead":     st.Ahead,
		"behind":    st.Behind,
		"staged":    st.Staged,
		"modified":  st.Modified,
		"untracked": st.Untracked,
		"changed":   st.Staged+st.Modified+st.Untracked > 0,
	}
	return res
}

func Checkout(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo, branch string) output.Result {
	start := time.Now()
	res := newResult(repo, "checkout")
	defer finish(&res, start)
	dir := ws.AbsPath(repo)
	if !isGitRepo(ctx, dir) {
		return skip(&res, output.CodeNotCloned, "repository not cloned")
	}
	st, err := parseStatus(ctx, dir)
	if err != nil {
		return fail(&res, output.CodeGitError, err)
	}
	res.Branch = st.Branch
	if st.Staged+st.Modified > 0 {
		res.Detail = map[string]any{"staged": st.Staged, "modified": st.Modified}
		return skip(&res, output.CodeDirtyWorktree, "uncommitted changes in worktree")
	}
	var action string
	switch {
	case refExists(ctx, dir, "refs/heads/"+branch):
		if _, err := run(ctx, dir, "checkout", branch); err != nil {
			return fail(&res, output.CodeGitError, err)
		}
		action = "switched"
	case refExists(ctx, dir, "refs/remotes/origin/"+branch):
		if _, err := run(ctx, dir, "checkout", "--track", "origin/"+branch); err != nil {
			return fail(&res, output.CodeGitError, err)
		}
		action = "created-tracking"
	default:
		_, src, base, err := ResolveDefaultBranch(ctx, dir, repo, ws.Repos.DefaultBranch)
		if err != nil {
			var dbe *DefaultBranchError
			if errors.As(err, &dbe) {
				res.Status = output.StatusFailed
				res.Error = &output.ErrInfo{
					Code:    output.CodeDefaultBranchUnresolved,
					Message: err.Error(),
					Hint:    "set defaultBranch explicitly in .barista/repos.json",
				}
				return res
			}
			return fail(&res, output.CodeGitError, err)
		}
		if _, err := run(ctx, dir, "checkout", "--no-track", "-b", branch, base); err != nil {
			return fail(&res, output.CodeGitError, err)
		}
		action = "created"
		res.Detail = map[string]any{"baseBranch": base, "defaultBranchSource": src}
	}
	res.Status = output.StatusOK
	res.Branch = branch
	if res.Detail == nil {
		res.Detail = map[string]any{}
	}
	res.Detail["action"] = action
	return res
}

func Fetch(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo, prune bool) output.Result {
	start := time.Now()
	res := newResult(repo, "fetch")
	defer finish(&res, start)
	dir := ws.AbsPath(repo)
	if !isGitRepo(ctx, dir) {
		return skip(&res, output.CodeNotCloned, "repository not cloned")
	}
	args := []string{"fetch"}
	if prune {
		args = append(args, "--prune")
	}
	if _, err := run(ctx, dir, args...); err != nil {
		return fail(&res, output.CodeGitError, err)
	}
	res.Status = output.StatusOK
	res.Branch = currentBranch(ctx, dir)
	res.Detail = map[string]any{"prune": prune}
	return res
}

func Pull(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo, rebase bool) output.Result {
	start := time.Now()
	res := newResult(repo, "pull")
	defer finish(&res, start)
	dir := ws.AbsPath(repo)
	if !isGitRepo(ctx, dir) {
		return skip(&res, output.CodeNotCloned, "repository not cloned")
	}
	args := []string{"pull"}
	if rebase {
		args = append(args, "--rebase")
	}
	if _, err := run(ctx, dir, args...); err != nil {
		return fail(&res, output.CodeGitError, err)
	}
	res.Status = output.StatusOK
	res.Branch = currentBranch(ctx, dir)
	res.Detail = map[string]any{"rebase": rebase}
	return res
}

func Push(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo, tags bool) output.Result {
	start := time.Now()
	res := newResult(repo, "push")
	defer finish(&res, start)
	dir := ws.AbsPath(repo)
	if !isGitRepo(ctx, dir) {
		return skip(&res, output.CodeNotCloned, "repository not cloned")
	}
	if tags {
		if _, err := run(ctx, dir, "push", "--tags"); err != nil {
			return fail(&res, output.CodeGitError, err)
		}
		res.Status = output.StatusOK
		res.Branch = currentBranch(ctx, dir)
		res.Detail = map[string]any{"tags": true}
		return res
	}
	st, err := parseStatus(ctx, dir)
	if err != nil {
		return fail(&res, output.CodeGitError, err)
	}
	res.Branch = st.Branch
	if st.HasUpstream && st.Ahead == 0 {
		return skip(&res, output.CodeNothingToPush, "no commits ahead of upstream")
	}
	if st.HasUpstream {
		if _, err := run(ctx, dir, "push"); err != nil {
			return fail(&res, output.CodeGitError, err)
		}
		res.Detail = map[string]any{"ahead": st.Ahead}
	} else {
		if _, err := run(ctx, dir, "push", "-u", "origin", st.Branch); err != nil {
			return fail(&res, output.CodeGitError, err)
		}
		res.Detail = map[string]any{"setUpstream": true}
	}
	res.Status = output.StatusOK
	return res
}
