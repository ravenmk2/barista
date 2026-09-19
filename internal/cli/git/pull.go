package git

import (
	"context"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func pullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull workspace repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			execute(cmd, "pull", func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
				rebase, _ := cmd.Flags().GetBool("rebase")
				if !cmd.Flags().Changed("rebase") {
					if v, ok := ws.Props.Bool("git.pull.rebase"); ok {
						rebase = v
					}
				}
				return gitrun.Pull(ctx, ws, repo, rebase)
			})
			return nil
		},
	}
	cmd.Flags().Bool("rebase", false, "rebase while pulling")
	return cmd
}
