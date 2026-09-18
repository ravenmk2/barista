package jdkcli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
)

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Delete a barista-installed JDK from disk and unregister it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			cfg, p, ok := userSettings(cmd)
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
					Code:    output.CodeJDKNotFound,
					Message: fmt.Sprintf("JDK %q is not registered", name),
				})
				return nil
			}
			res.Path = filepath.ToSlash(entry.Path)
			if !entry.Managed {
				failRes(&output.ErrInfo{
					Code:    output.CodeJDKNotManaged,
					Message: fmt.Sprintf("JDK %q was not installed by barista", name),
					Hint:    "to unregister without deleting files, run: barista jdk remove " + name,
				})
				return nil
			}
			root, err := jdk.InstallDir(cfg.InstallDir)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			managedDir := filepath.Join(root, name)
			rel, err := filepath.Rel(managedDir, entry.Path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				failRes(&output.ErrInfo{
					Code:    output.CodeJDKNotManaged,
					Message: fmt.Sprintf("registered path %s escapes the managed install dir", filepath.ToSlash(entry.Path)),
					Hint:    "fix jdk.json manually or run: barista jdk remove " + name,
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
				failRes(&output.ErrInfo{Code: output.CodeJDKUninstallFailed, Message: err.Error()})
				return nil
			}
			_, cleared, _ := reg.Remove(name)
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Detail = map[string]any{"deleted": !gone}
			if len(cleared) > 0 {
				res.Detail["clearedDefaults"] = cleared
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := "uninstalled " + p.Cyan(name)
				if gone {
					line += " (files already gone)"
				} else {
					line += fmt.Sprintf(" (deleted %s)", filepath.ToSlash(managedDir))
				}
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
	cmd.Flags().Bool("yes", false, "confirm deletion without prompting")
	return cmd
}
