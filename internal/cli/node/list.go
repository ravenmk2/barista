package nodecli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/node"
	"barista/internal/output"
)

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered Node.js installations",
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
			props, ok := workspaceProperties(cmd)
			if !ok {
				return nil
			}
			eff := effective(reg, props)
			defaultName := eff.Default
			if eff.DefaultSource == "workspace" {
				if e, _, re := reg.Resolve(eff.Default); re == nil {
					defaultName = e.Name
				}
			}
			results := make([]output.Result, len(reg.Installations))
			for i, e := range reg.Installations {
				detail := map[string]any{
					"version": e.Version,
					"managed": e.Managed,
				}
				if e.Name == defaultName && eff.Default != "" {
					detail["default"] = true
					detail["defaultSource"] = eff.DefaultSource
				}
				results[i] = output.Result{
					Name:   e.Name,
					Path:   filepath.ToSlash(e.Path),
					Status: output.StatusOK,
					Action: "list",
					Detail: detail,
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printList(reg, defaultName, eff.DefaultSource, p)
			}
			finish(cmd, results)
			return nil
		},
	}
}

func printList(reg *node.Registry, defaultName, defaultSource string, p output.Palette) {
	if len(reg.Installations) == 0 {
		fmt.Println(p.Dim("no Node.js installations registered"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tVERSION\tPATH\tTAGS")
	for _, e := range reg.Installations {
		var tags []string
		if e.Name == defaultName && defaultName != "" {
			tags = append(tags, "default:"+defaultSource)
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
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, e.Version, filepath.ToSlash(e.Path), tag)
	}
	_ = w.Flush()
}
