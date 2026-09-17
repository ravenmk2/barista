package cli

import (
	"github.com/spf13/cobra"

	gitcli "barista/internal/cli/git"
)

var ExitCode int

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "barista",
		Short: "Multi-repo workspace and dev environment manager",
	}
	root.PersistentFlags().Bool("json", false, "output results as JSON")
	root.PersistentFlags().Int("parallel", 10, "max concurrent repo operations")
	root.AddCommand(gitcli.NewCmd(&ExitCode))
	root.AddCommand(schemaCmd())
	return root
}
