package cli

import (
	"github.com/spf13/cobra"

	depscli "barista/internal/cli/deps"
	doctorcli "barista/internal/cli/doctor"
	gitcli "barista/internal/cli/git"
	gradlecli "barista/internal/cli/gradle"
	initcli "barista/internal/cli/init"
	javacli "barista/internal/cli/java"
	jdkcli "barista/internal/cli/jdk"
	mavencli "barista/internal/cli/maven"
	mvncli "barista/internal/cli/mvn"
	nodecli "barista/internal/cli/node"
	npmcli "barista/internal/cli/npm"
	repocli "barista/internal/cli/repo"
	upgradecli "barista/internal/cli/upgrade"
	uvcli "barista/internal/cli/uv"
)

var ExitCode int

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "barista",
		Short: "Multi-repo workspace and dev environment manager (" + version + ")",
	}
	root.PersistentFlags().Bool("json", false, "output results as JSON")
	root.PersistentFlags().Int("parallel", 10, "max concurrent repo operations")
	root.AddCommand(gitcli.NewCmd(&ExitCode))
	root.AddCommand(initcli.NewCmd(&ExitCode))
	root.AddCommand(jdkcli.NewCmd(&ExitCode))
	root.AddCommand(mavencli.NewCmd(&ExitCode))
	root.AddCommand(mvncli.NewCmd(&ExitCode))
	root.AddCommand(javacli.NewCmd(&ExitCode))
	root.AddCommand(gradlecli.NewCmd(&ExitCode))
	root.AddCommand(nodecli.NewCmd(&ExitCode))
	root.AddCommand(npmcli.NewNpmCmd(&ExitCode))
	root.AddCommand(npmcli.NewNpxCmd(&ExitCode))
	root.AddCommand(repocli.NewCmd(&ExitCode))
	root.AddCommand(depscli.NewCmd(&ExitCode))
	root.AddCommand(doctorcli.NewCmd(&ExitCode))
	root.AddCommand(upgradecli.NewCmd(&ExitCode, version))
	root.AddCommand(uvcli.NewCmd(&ExitCode))
	root.AddCommand(schemaCmd())
	return root
}
