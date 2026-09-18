package mavencli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/output"
)

func setJdkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-jdk <major|name>",
		Short: "Set the JDK used to run Maven (resolved against the jdk registry)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			scope, ok := scopeOf(cmd)
			if !ok {
				return nil
			}
			spec := args[0]
			jdkEntry, e := resolveJdkSpec(cmd, spec)
			if e != nil {
				failResult(cmd, p, output.Result{Name: spec, Action: "set-jdk", Status: output.StatusFailed, Error: e})
				return nil
			}
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			res := output.Result{
				Name:   spec,
				Status: output.StatusOK,
				Action: "set-jdk",
				Detail: map[string]any{
					"scope": scope,
					"resolved": map[string]any{
						"name":    jdkEntry.Name,
						"version": jdkEntry.Version,
						"path":    filepath.ToSlash(jdkEntry.Path),
					},
				},
			}
			if scope == "workspace" {
				if !writeWorkspaceProperty(cmd, "maven.jdk", spec) {
					return nil
				}
			} else {
				reg.SetJdk(spec)
				if !saveRegistry(cmd, reg, regPath) {
					return nil
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("jdk for maven set to %s (%s %s) [%s]\n", p.Cyan(spec), jdkEntry.Name, jdkEntry.Version, scope)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	addScopeFlag(cmd)
	return cmd
}
