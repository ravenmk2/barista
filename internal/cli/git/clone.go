package git

import (
	"context"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone",
		Short: "Clone workspace repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			execute(cmd, "clone", func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
				return gitrun.Clone(ctx, ws, repo)
			})
			return nil
		},
	}
}
