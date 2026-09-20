package depscli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/deps"
	"barista/internal/output"
	"barista/internal/workspace"
)

func orderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "order",
		Short: "Show the build order (topological levels; equal level = parallelizable)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			ws, g, repos, ok := loadSelection(cmd)
			if !ok {
				return nil
			}
			levels := g.Levels()
			cycles := g.Cycles()
			results := make([]output.Result, len(repos))
			for i, r := range repos {
				detail := map[string]any{"buildLevel": levels[r.Name]}
				if c := g.CycleOf(r.Name, cycles); len(c) > 0 {
					detail["cycle"] = c
				}
				results[i] = output.Result{
					Name:   r.Name,
					Path:   r.Path,
					Status: output.StatusOK,
					Action: "order",
					Detail: detail,
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printOrder(g, repos, levels, cycles, palette(ws))
			}
			finish(cmd, ws, results)
			return nil
		},
	}
}

func printOrder(g *deps.Graph, repos []workspace.Repo, levels map[string]int, cycles [][]string, p output.Palette) {
	sorted := make([]workspace.Repo, len(repos))
	copy(sorted, repos)
	sort.SliceStable(sorted, func(i, j int) bool {
		return levels[sorted[i].Name] < levels[sorted[j].Name]
	})
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, r := range sorted {
		line := fmt.Sprintf("%d\t%s", levels[r.Name], r.Name)
		if c := g.CycleOf(r.Name, cycles); len(c) > 0 {
			line += "\t" + p.Red("cycle: "+strings.Join(c, ", "))
		}
		_, _ = fmt.Fprintln(w, line)
	}
	_ = w.Flush()
}
