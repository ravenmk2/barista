package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/output"
	"barista/internal/runner"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Run git operations across workspace repositories",
	}
	cmd.PersistentFlags().StringSlice("label", nil, "select repos by label (repeatable, union)")
	cmd.PersistentFlags().StringSlice("repo", nil, "select repos by name (repeatable, union)")
	_ = cmd.RegisterFlagCompletionFunc("label", comp.Fn(comp.Labels))
	_ = cmd.RegisterFlagCompletionFunc("repo", comp.Fn(comp.RepoNames))
	cmd.AddCommand(
		cloneCmd(),
		statusCmd(),
		checkoutCmd(),
		fetchCmd(),
		pullCmd(),
		pushCmd(),
	)
	return cmd
}

type repoOp func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result

func execute(cmd *cobra.Command, action string, op repoOp) {
	*exitCode = 0
	jsonOut, _ := cmd.Flags().GetBool("json")
	command := strings.TrimPrefix(cmd.CommandPath(), "barista ")
	fail := func(e *output.ErrInfo) {
		fmt.Fprintf(os.Stderr, "barista: %s: %s\n", e.Code, e.Message)
		if e.Hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
		}
		if jsonOut {
			_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(command, e))
		}
		*exitCode = 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fail(&output.ErrInfo{Code: output.CodeWorkspaceNotFound, Message: err.Error()})
		return
	}
	ws, err := workspace.Load(cwd)
	if err != nil {
		var le *workspace.LoadError
		if errors.As(err, &le) {
			fail(&output.ErrInfo{Code: le.Code, Message: le.Message, Hint: le.Hint})
		} else {
			fail(&output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		}
		return
	}
	repos, selErr := selectRepos(cmd, ws)
	if selErr != nil {
		fail(selErr)
		return
	}
	userCfg, err := workspace.LoadUserConfig()
	if err != nil {
		fail(&output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		return
	}
	cfg := workspace.MergeConfig(userCfg, ws.Cfg)

	parallel := 10
	if cmd.Flags().Changed("parallel") {
		parallel, _ = cmd.Flags().GetInt("parallel")
	} else if cfg.Parallel > 0 {
		parallel = cfg.Parallel
	}
	if parallel < 1 {
		parallel = 1
	}

	tasks := make([]runner.Task[output.Result], len(repos))
	names := make([]string, len(repos))
	for i, repo := range repos {
		repo := repo
		names[i] = repo.Name
		tasks[i] = runner.Task[output.Result]{
			Name: repo.Name,
			Run: func(ctx context.Context) output.Result {
				return op(ctx, ws, repo)
			},
		}
	}
	ctx := cmd.Context()

	if jsonOut {
		results := runner.Run(ctx, tasks, parallel, nil)
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(command, filepath.ToSlash(ws.Root), results))
		*exitCode = output.ExitCode(results)
		return
	}
	color := output.ColorEnabled(cfg.Color)
	if output.StdoutIsTerminal() {
		panel := output.NewPanel(command, names, color)
		for i := range tasks {
			name, run := tasks[i].Name, tasks[i].Run
			tasks[i].Run = func(ctx context.Context) output.Result {
				panel.Start(name)
				return run(ctx)
			}
		}
		var mu sync.Mutex
		collected := make([]output.Result, len(tasks))
		go func() {
			<-panel.Started()
			runner.Run(ctx, tasks, parallel, func(i int, res output.Result) {
				mu.Lock()
				collected[i] = res
				mu.Unlock()
				panel.OnResult(i, res)
			})
			panel.Finish()
		}()
		_ = panel.Wait()
		mu.Lock()
		var final []output.Result
		for _, res := range collected {
			if res.Name != "" {
				final = append(final, res)
			}
		}
		mu.Unlock()
		output.NewTextRenderer(os.Stdout, command, color, output.NameWidth(names)).Finish(final)
		*exitCode = output.ExitCode(final)
		return
	}
	tr := output.NewTextRenderer(os.Stdout, command, color, output.NameWidth(names))
	results := runner.Run(ctx, tasks, parallel, tr.OnResult)
	tr.Finish(results)
	*exitCode = output.ExitCode(results)
}

func selectRepos(cmd *cobra.Command, ws *workspace.Workspace) ([]workspace.Repo, *output.ErrInfo) {
	labels, _ := cmd.Flags().GetStringSlice("label")
	names, _ := cmd.Flags().GetStringSlice("repo")
	if len(labels) == 0 && len(names) == 0 {
		if len(ws.Repos.Repos) == 0 {
			return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: "no repositories configured in .barista/repos.json"}
		}
		return ws.Repos.Repos, nil
	}
	labelSet := map[string]bool{}
	for _, l := range labels {
		labelSet[l] = true
	}
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	var matched []workspace.Repo
	for _, r := range ws.Repos.Repos {
		if nameSet[r.Name] {
			matched = append(matched, r)
			continue
		}
		for _, l := range r.Labels {
			if labelSet[l] {
				matched = append(matched, r)
				break
			}
		}
	}
	if len(matched) == 0 {
		return nil, &output.ErrInfo{
			Code:    output.CodeNoMatchingRepos,
			Message: "no repositories match the given --label/--repo filters",
			Hint:    "check for typos in the filter values",
		}
	}
	return matched, nil
}
