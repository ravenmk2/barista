package nodecli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/node"
	"barista/internal/output"
)

func setDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "set-default <name>",
		ValidArgsFunction: comp.Fn(comp.NodeNames),
		Short:             "Set the user-level default Node.js installation (workspace override: barista node use)",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if !node.ValidName(args[0]) {
				return fmt.Errorf("invalid node name %q (want [a-z0-9][a-z0-9._-]*)", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			name := args[0]
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			entry, e := reg.SetDefault(name)
			if e != nil {
				failResult(cmd, p, output.Result{Name: name, Action: "set-default", Status: output.StatusFailed, Error: e})
				return nil
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res := output.Result{
				Name:   name,
				Path:   filepath.ToSlash(entry.Path),
				Status: output.StatusOK,
				Action: "set-default",
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("default set to %s\n", p.Cyan(name))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
