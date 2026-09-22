package npmcli

import (
	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	nodecli "barista/internal/cli/node"
)

func NewNpmCmd(exit *int) *cobra.Command {
	return toolCmd(exit, "npm", "install")
}

func NewNpxCmd(exit *int) *cobra.Command {
	return toolCmd(exit, "npx", "vite --version")
}

func toolCmd(exit *int, tool, exampleArgs string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   tool + " [flags] -- <" + tool + " args...>",
		Short:                 "Run " + tool + " with the workspace-aware Node.js",
		DisableFlagsInUseLine: true,
		Long: "Run " + tool + " in the current directory. Arguments after \"--\" are passed through verbatim.\n" +
			"Node.js resolution (high to low): --node > .node-version/.nvmrc file > repo properties[\"node\"] > workspace node > user node.json default > ambient PATH.\n" +
			"File detection can be disabled with the workspace property detect.files=false.\n" +
			"When a registered installation is resolved, the child process gets NODE_HOME plus the installation bin dir prepended to PATH; on Windows the .cmd shim runs via cmd /c.",
		Example: "  barista " + tool + " -- " + exampleArgs + "\n" +
			"  barista " + tool + " --node 22 --dry-run -- " + exampleArgs,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			nodecli.RunToolExec(cmd, exit, tool, args)
			return nil
		},
	}
	cmd.Flags().String("node", "", "Node.js spec (registry name or version) used to run "+tool+"; overrides every config level")
	_ = cmd.RegisterFlagCompletionFunc("node", comp.Fn(comp.NodeSpecs))
	cmd.Flags().Bool("dry-run", false, "print the resolved Node.js and full command line without executing")
	return cmd
}
