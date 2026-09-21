package depscli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/deps"
	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Show inter-repo dependencies declared in the manifest",
	}
	cmd.PersistentFlags().StringSlice("label", nil, "select repos by label (repeatable, union)")
	cmd.PersistentFlags().StringSlice("repo", nil, "select repos by name (repeatable, union)")
	_ = cmd.RegisterFlagCompletionFunc("label", comp.Fn(comp.Labels))
	_ = cmd.RegisterFlagCompletionFunc("repo", comp.Fn(comp.RepoNames))
	cmd.AddCommand(showCmd(), orderCmd())
	return cmd
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
}

func palette(ws *workspace.Workspace) output.Palette {
	user, err := workspace.LoadUserConfig()
	if err != nil {
		user = workspace.ConfigFile{}
	}
	cfg := workspace.MergeConfig(user, ws.Cfg)
	return output.NewPalette(output.ColorEnabled(cfg.Color), cfg.ColorProfile)
}

func finish(cmd *cobra.Command, ws *workspace.Workspace, results []output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), filepath.ToSlash(ws.Root), results))
	}
	*exitCode = output.ExitCode(results)
}

func fail(cmd *cobra.Command, e *output.ErrInfo) {
	fmt.Fprintf(os.Stderr, "barista: %s: %s\n", e.Code, e.Message)
	if e.Hint != "" {
		fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
	}
	*exitCode = 2
}

func loadWorkspace(cmd *cobra.Command) (*workspace.Workspace, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		fail(cmd, &output.ErrInfo{Code: output.CodeWorkspaceNotFound, Message: err.Error()})
		return nil, false
	}
	ws, err := workspace.Load(cwd)
	if err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		fail(cmd, e)
		return nil, false
	}
	return ws, true
}

// loadSelection loads the workspace, builds the dependency graph over the
// full manifest, and returns the selected repos in declaration order.
func loadSelection(cmd *cobra.Command) (*workspace.Workspace, *deps.Graph, []workspace.Repo, bool) {
	ws, ok := loadWorkspace(cmd)
	if !ok {
		return nil, nil, nil, false
	}
	repos, selErr := selectRepos(cmd, ws)
	if selErr != nil {
		fail(cmd, selErr)
		return nil, nil, nil, false
	}
	return ws, deps.Build(ws.Repos.Repos), repos, true
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
