package depscli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/deps"
	"barista/internal/output"
	"barista/internal/workspace"
)

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the dependency adjacency list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			ws, g, repos, ok := loadSelection(cmd)
			if !ok {
				return nil
			}
			cycles := g.Cycles()
			results := make([]output.Result, len(repos))
			for i, r := range repos {
				detail := map[string]any{
					"deps":       nonNil(g.Deps(r.Name)),
					"dependedBy": nonNil(g.DependedBy(r.Name)),
				}
				if d := g.Dangling(r.Name); len(d) > 0 {
					detail["dangling"] = d
				}
				if c := g.CycleOf(r.Name, cycles); len(c) > 0 {
					detail["cycle"] = c
				}
				results[i] = output.Result{
					Name:   r.Name,
					Path:   r.Path,
					Status: output.StatusOK,
					Action: "show",
					Detail: detail,
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printShow(g, repos, cycles, palette(ws))
			}
			finish(cmd, ws, results)
			return nil
		},
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func printShow(g *deps.Graph, repos []workspace.Repo, cycles [][]string, p output.Palette) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, r := range repos {
		var marks []string
		if d := g.Dangling(r.Name); len(d) > 0 {
			marks = append(marks, p.Yellow("unknown: "+strings.Join(d, ", ")))
		}
		if c := g.CycleOf(r.Name, cycles); len(c) > 0 {
			marks = append(marks, p.Red("cycle: "+strings.Join(c, ", ")))
		}
		arrow := p.Dim("-")
		if d := g.Deps(r.Name); len(d) > 0 {
			arrow = "→ " + strings.Join(d, ", ")
		}
		if len(marks) > 0 {
			arrow += "  " + strings.Join(marks, "  ")
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\n", r.Name, arrow)
	}
	_ = w.Flush()
}
