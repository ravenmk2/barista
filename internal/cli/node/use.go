package nodecli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/output"
	"barista/internal/workspace"
)

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "use <name|version>",
		ValidArgsFunction: comp.Fn(comp.NodeSpecs),
		Short:             `Set the Node.js for the current workspace (writes .barista/properties.json "node")`,
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			cwd, err := os.Getwd()
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			root := workspace.FindWorkspaceRoot(cwd)
			if root == "" {
				fail(cmd, &output.ErrInfo{
					Code:    output.CodeWorkspaceNotFound,
					Message: "no .barista workspace found in current directory or any parent",
					Hint:    "run inside a barista workspace",
				})
				return nil
			}
			spec := args[0]
			reg, _, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			entry, _, e := reg.Resolve(spec)
			if e != nil {
				failResult(cmd, p, output.Result{Name: spec, Action: "use", Status: output.StatusFailed, Error: e})
				return nil
			}
			cfgPath := filepath.Join(root, ".barista", "properties.json")
			if err := workspace.SetProperty(cfgPath, "node", spec); err != nil {
				fail(cmd, configErrInfo(err))
				return nil
			}
			res := output.Result{
				Name:   spec,
				Path:   filepath.ToSlash(entry.Path),
				Status: output.StatusOK,
				Action: "use",
				Detail: map[string]any{
					"spec":      spec,
					"workspace": filepath.ToSlash(root),
					"resolved": map[string]any{
						"name":    entry.Name,
						"version": entry.Version,
						"path":    filepath.ToSlash(entry.Path),
					},
				},
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("workspace node set to %s (%s %s) [%s]\n", p.Cyan(spec), entry.Name, entry.Version, filepath.ToSlash(root))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
