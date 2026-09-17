package git

import (
	"context"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of workspace repositories (offline)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			execute(cmd, "status", func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
				return gitrun.Status(ctx, ws, repo)
			})
			return nil
		},
	}
}
