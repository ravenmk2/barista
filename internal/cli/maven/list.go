package mavencli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/maven"
	"barista/internal/output"
)

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered Maven installations",
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
			wsCfg, ok := workspaceConfig(cmd)
			if !ok {
				return nil
			}
			eff := effective(reg, wsCfg)
			results := make([]output.Result, len(reg.Installations))
			for i, e := range reg.Installations {
				detail := map[string]any{
					"version": e.Version,
					"managed": e.Managed,
				}
				if e.Name == eff.Default {
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
				printList(reg, eff, p)
			}
			finish(cmd, results)
			return nil
		},
	}
}

func printList(reg *maven.Registry, eff effectiveSettings, p output.Palette) {
	if len(reg.Installations) == 0 {
		fmt.Println(p.Dim("no Maven installations registered"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tVERSION\tPATH\tTAGS")
	for _, e := range reg.Installations {
		var tags []string
		if e.Name == eff.Default {
			tags = append(tags, "default:"+eff.DefaultSource)
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
