package cli

import (
	"github.com/spf13/cobra"

	doctorcli "barista/internal/cli/doctor"
	gitcli "barista/internal/cli/git"
	initcli "barista/internal/cli/init"
	javacli "barista/internal/cli/java"
	jdkcli "barista/internal/cli/jdk"
	mavencli "barista/internal/cli/maven"
	mvncli "barista/internal/cli/mvn"
	repocli "barista/internal/cli/repo"
	upgradecli "barista/internal/cli/upgrade"
)

var ExitCode int

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "barista",
		Short: "Multi-repo workspace and dev environment manager",
	}
	root.PersistentFlags().Bool("json", false, "output results as JSON")
	root.PersistentFlags().Int("parallel", 10, "max concurrent repo operations")
	root.AddCommand(gitcli.NewCmd(&ExitCode))
	root.AddCommand(initcli.NewCmd(&ExitCode))
	root.AddCommand(jdkcli.NewCmd(&ExitCode))
	root.AddCommand(mavencli.NewCmd(&ExitCode))
	root.AddCommand(mvncli.NewCmd(&ExitCode))
	root.AddCommand(javacli.NewCmd(&ExitCode))
	root.AddCommand(repocli.NewCmd(&ExitCode))
	root.AddCommand(doctorcli.NewCmd(&ExitCode))
	root.AddCommand(upgradecli.NewCmd(&ExitCode, version))
	root.AddCommand(schemaCmd())
	return root
}
