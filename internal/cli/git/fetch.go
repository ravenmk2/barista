package git

import (
	"context"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func fetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch workspace repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			execute(cmd, "fetch", func(ctx context.Context, ws *workspace.Workspace, repo workspace.Repo) output.Result {
				prune, _ := cmd.Flags().GetBool("prune")
				if !cmd.Flags().Changed("prune") {
					if v, ok := ws.Props.Bool("git.fetch.prune"); ok {
						prune = v
					}
				}
				return gitrun.Fetch(ctx, ws, repo, prune)
			})
			return nil
		},
	}
	cmd.Flags().Bool("prune", false, "prune remote-tracking refs while fetching")
	return cmd
}
