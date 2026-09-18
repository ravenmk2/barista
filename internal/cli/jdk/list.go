package jdkcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
)

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered JDKs",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			reg, _, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			results := make([]output.Result, len(reg.JDKs))
			for i, e := range reg.JDKs {
				results[i] = output.Result{
					Name:   e.Name,
					Path:   filepath.ToSlash(e.Path),
					Status: output.StatusOK,
					Action: "list",
					Detail: map[string]any{
						"major":      e.Major,
						"version":    e.Version,
						"managed":    e.Managed,
						"defaultFor": reg.DefaultMajors(e.Name),
					},
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printList(reg, p)
			}
			finish(cmd, results)
			return nil
		},
	}
}

func printList(reg *jdk.Registry, p output.Palette) {
	if len(reg.JDKs) == 0 {
		fmt.Println(p.Dim("no JDKs registered"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tMAJOR\tVERSION\tPATH\tTAGS")
	for _, e := range reg.JDKs {
		var tags []string
		for _, m := range reg.DefaultMajors(e.Name) {
			tags = append(tags, fmt.Sprintf("default:%d", m))
		}
		if e.Managed {
			tags = append(tags, "managed")
		}
		tag := strings.Join(tags, ",")
		if tag == "" {
			tag = "-"
		} else {
			tag = p.Yellow(tag)
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", e.Name, e.Major, e.Version, filepath.ToSlash(e.Path), tag)
	}
	_ = w.Flush()
}
