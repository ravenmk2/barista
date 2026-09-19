package mavencli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/output"
)

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "remove <name>",
		ValidArgsFunction: comp.Fn(comp.MavenNames),
		Short:             "Unregister a Maven installation (keeps files on disk)",
		Args:              cobra.ExactArgs(1),
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
			entry, clearedDefault, e := reg.Remove(name)
			if e != nil {
				failResult(cmd, p, output.Result{Name: name, Action: "remove", Status: output.StatusFailed, Error: e})
				return nil
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res := output.Result{
				Name:   name,
				Path:   filepath.ToSlash(entry.Path),
				Status: output.StatusOK,
				Action: "remove",
			}
			if clearedDefault {
				res.Detail = map[string]any{"clearedDefault": true}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := "removed " + p.Cyan(name)
				if clearedDefault {
					line += " (cleared default)"
				}
				fmt.Println(line)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
