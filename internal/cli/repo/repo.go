package repocli

import (
	"errors"
	"fmt"
	"os"
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
	cmd.AddCommand(addCmd())
	return cmd
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
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
