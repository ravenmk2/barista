package git

import (
	"context"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func pushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push workspace repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			execute(cmd, "push", func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
				tags, _ := cmd.Flags().GetBool("tags")
				return gitrun.Push(ctx, ws, repo, tags)
			})
			return nil
		},
	}
	cmd.Flags().Bool("tags", false, "push tags")
	return cmd
}
