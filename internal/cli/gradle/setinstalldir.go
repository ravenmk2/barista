package gradlecli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/gradle"
	"barista/internal/output"
	"barista/internal/workspace"
)

func setInstallDirCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-install-dir <path>",
		Short: "Set the managed Gradle install root (gradle.json installDir); --reset restores the builtin default",
		Args: func(cmd *cobra.Command, args []string) error {
			reset, _ := cmd.Flags().GetBool("reset")
			if reset && len(args) > 0 {
				return fmt.Errorf("--reset does not take a <path> argument")
			}
			if !reset {
				return cobra.ExactArgs(1)(cmd, args)
			}
			return nil
		},
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
			res := output.Result{Name: "installDir", Action: "set-install-dir"}
			if reset, _ := cmd.Flags().GetBool("reset"); reset {
				reg.InstallDir = ""
				if !saveRegistry(cmd, reg, regPath) {
					return nil
				}
				def, err := gradle.DefaultInstallDir()
				if err != nil {
					fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
					return nil
				}
				res.Status = output.StatusOK
				res.Detail = map[string]any{"value": filepath.ToSlash(def), "source": "builtin"}
				if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
					fmt.Printf("install dir reset to builtin default %s\n", p.Cyan(filepath.ToSlash(def)))
				}
				finish(cmd, []output.Result{res})
				return nil
			}
			dir := workspace.ExpandHome(args[0])
			if !filepath.IsAbs(dir) {
				fail(cmd, &output.ErrInfo{
					Code:    output.CodeConfigError,
					Message: fmt.Sprintf("install dir %q must be an absolute path (~ allowed)", args[0]),
				})
				return nil
			}
			reg.InstallDir = dir
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Path = filepath.ToSlash(dir)
			res.Detail = map[string]any{"value": filepath.ToSlash(dir), "source": "user"}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("install dir set to %s\n", p.Cyan(filepath.ToSlash(dir)))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().Bool("reset", false, "clear installDir and restore the builtin default (~/.barista/toolchains/gradle)")
	return cmd
}
