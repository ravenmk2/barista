package gradlecli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/gradle"
	"barista/internal/output"
)

func setDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set-default <name>",
		ValidArgsFunction: comp.Fn(comp.GradleNames),
		Short:             "Set the default Gradle installation",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if !gradle.ValidName(args[0]) {
				return fmt.Errorf("invalid gradle name %q (want [a-z0-9][a-z0-9._-]*)", args[0])
			}
			return nil
		},
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
			res := output.Result{
				Name:   name,
				Path:   filepath.ToSlash(entry.Path),
				Status: output.StatusOK,
				Action: "set-default",
				Detail: map[string]any{"scope": scope},
			}
			if scope == "workspace" {
				if !writeWorkspaceProperty(cmd, "gradle.default", name) {
					return nil
				}
			} else if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("default set to %s [%s]\n", p.Cyan(name), scope)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	addScopeFlag(cmd)
	return cmd
}
