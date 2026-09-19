package mavencli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/maven"
	"barista/internal/output"
)

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "uninstall <name>",
		ValidArgsFunction: comp.Fn(comp.MavenNames),
		Short:             "Delete a barista-installed Maven from disk and unregister it",
		Args:              cobra.ExactArgs(1),
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
			name := args[0]
			res := output.Result{Name: name, Action: "uninstall"}
			failRes := func(e *output.ErrInfo) {
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, p, res)
			}
			entry := reg.Find(name)
			if entry == nil {
				failRes(&output.ErrInfo{
					Code:    output.CodeMavenNotFound,
					Message: fmt.Sprintf("maven %q is not registered", name),
				})
				return nil
			}
			res.Path = filepath.ToSlash(entry.Path)
			if !entry.Managed {
				failRes(&output.ErrInfo{
					Code:    output.CodeMavenNotManaged,
					Message: fmt.Sprintf("maven %q was not installed by barista", name),
					Hint:    "to unregister without deleting files, run: barista maven remove " + name,
				})
				return nil
			}
			root, err := maven.InstallDir(reg.InstallDir)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			managedDir := filepath.Join(root, name)
			rel, err := filepath.Rel(managedDir, entry.Path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				failRes(&output.ErrInfo{
					Code:    output.CodeMavenNotManaged,
					Message: fmt.Sprintf("registered path %s escapes the managed install dir", filepath.ToSlash(entry.Path)),
					Hint:    "fix maven.json manually or run: barista maven remove " + name,
				})
				return nil
			}
			confirmed, _ := cmd.Flags().GetBool("yes")
			if !confirmed {
				if !output.StdinIsTerminal() {
					fail(cmd, &output.ErrInfo{
						Code:    output.CodeConfirmationRequired,
						Message: fmt.Sprintf("uninstalling %q deletes %s", name, filepath.ToSlash(managedDir)),
						Hint:    "pass --yes to confirm",
					})
					return nil
				}
				fmt.Fprintf(os.Stderr, "delete %s? [y/N] ", p.Red(filepath.ToSlash(managedDir)))
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				if a := strings.TrimSpace(line); !strings.EqualFold(a, "y") && !strings.EqualFold(a, "yes") {
					fmt.Println(p.Dim("aborted"))
					res.Status = output.StatusSkipped
					res.Detail = map[string]any{"reason": "aborted"}
					finish(cmd, []output.Result{res})
					return nil
				}
			}
			gone := false
			if _, err := os.Stat(managedDir); os.IsNotExist(err) {
				gone = true
			}
			if err := os.RemoveAll(managedDir); err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenUninstallFailed, Message: err.Error()})
				return nil
			}
			_, clearedDefault, _ := reg.Remove(name)
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Detail = map[string]any{"deleted": !gone}
			if clearedDefault {
				res.Detail["clearedDefault"] = true
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := "uninstalled " + p.Cyan(name)
				if gone {
					line += " (files already gone)"
				} else {
					line += fmt.Sprintf(" (deleted %s)", filepath.ToSlash(managedDir))
				}
				if clearedDefault {
					line += " (cleared default)"
				}
				fmt.Println(line)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "confirm deletion without prompting")
	return cmd
}
