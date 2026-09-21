package mavencli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Register an installed Maven (reads the version from lib/maven-core-*.jar)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			path := workspace.ExpandHome(args[0])
			res := output.Result{Action: "add"}
			failRes := func(e *output.ErrInfo) {
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, p, res)
			}
			info, e := maven.Probe(path)
			if e != nil {
				res.Path = filepath.ToSlash(path)
				failRes(e)
				return nil
			}
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				name = autoName(reg, info.Version)
			} else if e := customNameError(name); e != nil {
				failRes(e)
				return nil
			}
			res.Name = name
			res.Path = filepath.ToSlash(info.Home)
			if e := reg.Add(maven.Entry{Name: name, Version: info.Version, Path: info.Home}); e != nil {
				failRes(e)
				return nil
			}
			setDefault, _ := cmd.Flags().GetBool("default")
			if setDefault {
				_, _ = reg.SetDefault(name)
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Detail = map[string]any{
				"version": info.Version,
				"managed": false,
			}
			if setDefault {
				res.Detail["default"] = true
			}
			for _, other := range reg.Installations {
				if other.Name != name && other.Path == info.Home {
					res.Detail["samePathAs"] = other.Name
					break
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := fmt.Sprintf("added %s (%s) at %s", p.Cyan(name), info.Version, filepath.ToSlash(info.Home))
				if setDefault {
					line += " (default)"
				}
				fmt.Println(line)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().String("name", "", "register under this name (default: maven-<version> from the probed version)")
	cmd.Flags().Bool("default", false, "set as the default maven")
	return cmd
}
