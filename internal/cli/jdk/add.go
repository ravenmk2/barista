package jdkcli

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Register an installed JDK (probes java/javac for version info)",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			if !jdk.ValidName(args[0]) {
				return fmt.Errorf("invalid JDK name %q (want [a-z0-9][a-z0-9._-]*)", args[0])
			}
			if _, err := strconv.Atoi(args[0]); err == nil {
				return fmt.Errorf("JDK name %q must not be a bare number (ambiguous with major version in barista jdk which)", args[0])
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
			res := output.Result{Name: name, Action: "add"}
			if e := reg.Find(name); e != nil {
				res.Status = output.StatusFailed
				res.Error = &output.ErrInfo{
					Code:    output.CodeJDKExists,
					Message: fmt.Sprintf("JDK %q is already registered", name),
					Hint:    "choose another name or run: barista jdk remove " + name,
				}
				failResult(cmd, p, res)
				return nil
			}
			path := workspace.ExpandHome(args[1])
			info, e := jdk.Probe(path)
			if e != nil {
				res.Path = filepath.ToSlash(path)
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, p, res)
				return nil
			}
			res.Path = filepath.ToSlash(info.Home)
			setDefault, _ := cmd.Flags().GetBool("default")
			if e := reg.Add(jdk.Entry{Name: name, Major: info.Major, Version: info.Version, Path: info.Home}); e != nil {
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, p, res)
				return nil
			}
			if setDefault {
				_, _ = reg.SetDefault(info.Major, name)
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Detail = map[string]any{
				"major":   info.Major,
				"version": info.Version,
				"distro":  info.Distro,
				"managed": false,
			}
			if setDefault {
				res.Detail["defaultFor"] = []int{info.Major}
			}
			for _, other := range reg.JDKs {
				if other.Name != name && other.Path == info.Home {
					res.Detail["samePathAs"] = other.Name
					break
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := fmt.Sprintf("added %s (%s %s) at %s", p.Cyan(name), info.Distro, info.Version, filepath.ToSlash(info.Home))
				if setDefault {
					line += fmt.Sprintf(" (default for %d)", info.Major)
				}
				fmt.Println(line)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().Bool("default", false, "set as the default for its major version")
	return cmd
}
