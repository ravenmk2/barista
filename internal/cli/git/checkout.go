package git

import (
	"context"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func checkoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "checkout <branch>",
		Short: "Checkout or create a branch in workspace repositories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]
			execute(cmd, "checkout", func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
				return gitrun.Checkout(ctx, ws, repo, branch)
			})
			return nil
		},
	}
}
