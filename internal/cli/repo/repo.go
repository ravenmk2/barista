package repocli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage the workspace repository manifest",
	}
	cmd.AddCommand(addCmd(), listCmd(), removeCmd())
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
	return output.NewPalette(output.ColorEnabled(workspace.MergeConfig(user, ws.Cfg).Color))
}

func finish(cmd *cobra.Command, ws *workspace.Workspace, results []output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), filepath.ToSlash(ws.Root), results))
	}
	*exitCode = output.ExitCode(results)
}

func failResult(cmd *cobra.Command, ws *workspace.Workspace, res output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
		fmt.Fprintf(os.Stderr, "barista: %s: %s\n", res.Error.Code, res.Error.Message)
		if res.Error.Hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", res.Error.Hint)
		}
	}
	finish(cmd, ws, []output.Result{res})
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
