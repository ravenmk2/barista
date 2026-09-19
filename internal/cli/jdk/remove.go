package jdkcli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/output"
)

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "remove <name>",
		ValidArgsFunction: comp.Fn(comp.JdkNames),
		Short:             "Unregister a JDK (keeps files on disk)",
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
			entry, cleared, e := reg.Remove(name)
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
			if len(cleared) > 0 {
				res.Detail = map[string]any{"clearedDefaults": cleared}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := "removed " + p.Cyan(name)
				if len(cleared) > 0 {
					majors := make([]string, len(cleared))
					for i, m := range cleared {
						majors[i] = strconv.Itoa(m)
					}
					line += fmt.Sprintf(" (cleared default for %s)", strings.Join(majors, ", "))
				}
				fmt.Println(line)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
