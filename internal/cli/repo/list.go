package repocli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/output"
	"barista/internal/workspace"
)

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List repositories in the manifest",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			ws, ok := loadWorkspace(cmd)
			if !ok {
				return nil
			}
			results := make([]output.Result, len(ws.Repos.Repos))
			for i, r := range ws.Repos.Repos {
				res := output.Result{
					Name:   r.Name,
					Path:   r.Path,
					Status: output.StatusOK,
					Action: "list",
					Detail: map[string]any{
						"url":         r.URL,
						"resolvedUrl": r.ResolvedURL,
						"cloned":      isCheckout(ws.AbsPath(r)),
					},
				}
				if len(r.Labels) > 0 {
					res.Detail["labels"] = r.Labels
				}
				if r.DefaultBranch != "" {
					res.Detail["defaultBranch"] = r.DefaultBranch
				}
				if len(r.Properties) > 0 {
					res.Detail["properties"] = r.Properties
				}
				results[i] = res
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printList(ws, results, palette(ws))
			}
			finish(cmd, ws, results)
			return nil
		},
	}
}

func isCheckout(abs string) bool {
	_, err := os.Stat(filepath.Join(abs, ".git"))
	return err == nil
}

func printList(ws *workspace.Workspace, results []output.Result, p output.Palette) {
	if len(results) == 0 {
		fmt.Println(p.Dim("no repos registered"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPATH\tURL\tLABELS\tCHECKOUT")
	for i, r := range ws.Repos.Repos {
		labels := strings.Join(r.Labels, ",")
		if labels == "" {
			labels = "-"
		}
		checkout := "cloned"
		if c, _ := results[i].Detail["cloned"].(bool); !c {
			checkout = p.Yellow("not cloned")
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Name, filepath.ToSlash(r.Path), r.ResolvedURL, labels, checkout)
	}
	_ = w.Flush()
}
