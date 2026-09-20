package jdkcli

import (
	"errors"
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
		Use:               "use <major|name>",
		ValidArgsFunction: comp.Fn(comp.JdkSpecs),
		Short:             `Set the JDK for the current workspace (writes .barista/properties.json "jdk", plus .java-version at the checkout root when inside one)`,
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
			if err := workspace.SetProperty(cfgPath, "jdk", spec); err != nil {
				ce := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
				var le *workspace.LoadError
				if errors.As(err, &le) {
					ce.Code, ce.Message, ce.Hint = le.Code, le.Message, le.Hint
				}
				fail(cmd, ce)
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
			javaVersionNote := ""
			if gitRoot, ok := workspace.FindGitRoot(cwd, root); ok {
				prev, err := workspace.WriteJavaVersionFile(gitRoot, entry.Version)
				if err != nil {
					res.Status = output.StatusFailed
					res.Error = &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
					failResult(cmd, p, res)
					return nil
				}
				file := filepath.ToSlash(filepath.Join(gitRoot, ".java-version"))
				res.Detail["javaVersionFile"] = file
				if prev != entry.Version {
					if prev != "" {
						res.Detail["previous"] = prev
						javaVersionNote = fmt.Sprintf("updated %s (%s → %s)\n", file, prev, entry.Version)
					} else {
						javaVersionNote = fmt.Sprintf("updated %s (%s)\n", file, entry.Version)
					}
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("workspace jdk set to %s (%s %s) [%s]\n", p.Cyan(spec), entry.Name, entry.Version, filepath.ToSlash(root))
				if javaVersionNote != "" {
					fmt.Print(javaVersionNote)
				}
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
