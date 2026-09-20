package repocli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/output"
	"barista/internal/workspace"
)

func removeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove <name>",
		ValidArgsFunction: comp.Fn(comp.RepoNames),
		Short:             "Unregister a repository (keeps the checkout on disk)",
		Long: "Unregister a repository from .barista/repos.json. The checkout stays on disk\n" +
			"unless --delete is given, which removes it after confirmation (requires the path\n" +
			"to be a git checkout).",
		Example: `  barista repo remove order-service
  barista repo remove order-service --delete --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			ws, ok := loadWorkspace(cmd)
			if !ok {
				return nil
			}
			name := args[0]
			res := output.Result{Name: name, Action: "remove"}
			failRes := func(e *output.ErrInfo) {
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, ws, res)
			}
			var entry *workspace.Repo
			for i := range ws.Repos.Repos {
				if ws.Repos.Repos[i].Name == name {
					entry = &ws.Repos.Repos[i]
					break
				}
			}
			if entry == nil {
				failRes(&output.ErrInfo{
					Code:    output.CodeRepoNotFound,
					Message: fmt.Sprintf("repo %q is not in the manifest", name),
				})
				return nil
			}
			res.Path = entry.Path
			deleted := false
			del, _ := cmd.Flags().GetBool("delete")
			if del {
				abs := ws.AbsPath(*entry)
				exists := false
				if _, err := os.Stat(abs); err == nil {
					exists = true
				}
				if exists && !isCheckout(abs) {
					failRes(&output.ErrInfo{
						Code:    output.CodeNotCloned,
						Message: fmt.Sprintf("%s exists but is not a git checkout; refusing to delete it", filepath.ToSlash(abs)),
						Hint:    "delete it manually or run without --delete",
					})
					return nil
				}
				if exists {
					if confirmed, _ := cmd.Flags().GetBool("yes"); !confirmed {
						if !output.StdinIsTerminal() {
							fail(cmd, &output.ErrInfo{
								Code:    output.CodeConfirmationRequired,
								Message: fmt.Sprintf("removing %q with --delete deletes %s", name, filepath.ToSlash(abs)),
								Hint:    "pass --yes to confirm",
							})
							return nil
						}
						fmt.Fprintf(os.Stderr, "delete %s? [y/N] ", palette(ws).Red(filepath.ToSlash(abs)))
						line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
						if a := strings.TrimSpace(line); !strings.EqualFold(a, "y") && !strings.EqualFold(a, "yes") {
							fmt.Println(palette(ws).Dim("aborted"))
							res.Status = output.StatusSkipped
							res.Detail = map[string]any{"reason": "aborted"}
							finish(cmd, ws, []output.Result{res})
							return nil
						}
					}
					if err := os.RemoveAll(abs); err != nil {
						failRes(&output.ErrInfo{
							Code:    output.CodeRepoDeleteFailed,
							Message: fmt.Sprintf("cannot delete %s: %v", filepath.ToSlash(abs), err),
						})
						return nil
					}
					deleted = true
				}
			}
			if _, removed, err := workspace.RemoveRepo(ws.Root, name); err != nil {
				e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
				var le *workspace.LoadError
				if errors.As(err, &le) {
					e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
				}
				fail(cmd, e)
				return nil
			} else if !removed {
				failRes(&output.ErrInfo{
					Code:    output.CodeRepoNotFound,
					Message: fmt.Sprintf("repo %q is not in the manifest", name),
				})
				return nil
			}
			res.Status = output.StatusOK
			res.Detail = map[string]any{"deleted": deleted}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				line := "removed " + name
				switch {
				case deleted:
					line += fmt.Sprintf(" (deleted %s)", filepath.ToSlash(res.Path))
				case del:
					line += " (no checkout on disk)"
				default:
					line += fmt.Sprintf(" (checkout kept at %s)", filepath.ToSlash(res.Path))
				}
				fmt.Println(line)
			}
			finish(cmd, ws, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().Bool("delete", false, "also delete the checkout directory (must be a git checkout)")
	cmd.Flags().Bool("yes", false, "confirm deletion without prompting (with --delete)")
	return cmd
}
