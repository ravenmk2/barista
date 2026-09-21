package mavencli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/maven"
	"barista/internal/output"
)

func availableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "available",
		Short: "List Maven versions available for download from the Apache archive",
		Args:  cobra.NoArgs,
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
			versions, e := maven.Available(cmd.Context())
			if e != nil {
				failResult(cmd, p, output.Result{Name: "apache-archive", Action: "available", Status: output.StatusFailed, Error: e})
				return nil
			}
			if all, _ := cmd.Flags().GetBool("all"); !all {
				versions = maven.LatestPerMinor(versions)
			}
			results := make([]output.Result, 0, len(versions))
			for _, v := range versions {
				installed := false
				for _, inst := range reg.Installations {
					if maven.CompareVersions(inst.Version, v.Version) == 0 {
						installed = true
						break
					}
				}
				results = append(results, output.Result{
					Name:   maven.NameFor(v.Version),
					Status: output.StatusOK,
					Action: "available",
					Detail: map[string]any{
						"version":   v.Version,
						"latest":    v.Latest,
						"installed": installed,
					},
				})
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printAvailable(results, p)
			}
			finish(cmd, results)
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "list every version (default: latest of each minor line)")
	return cmd
}

func printAvailable(results []output.Result, p output.Palette) {
	if len(results) == 0 {
		fmt.Println(p.Dim("no releases available"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "VERSION\tTAGS")
	for _, res := range results {
		d := res.Detail
		var tags []string
		if d["latest"] == true {
			tags = append(tags, "latest")
		}
		if d["installed"] == true {
			tags = append(tags, "installed")
		}
		tag := strings.Join(tags, ",")
		if tag == "" {
			tag = "-"
		} else {
			tag = p.Yellow(tag)
		}
		_, _ = fmt.Fprintf(w, "%v\t%s\n", d["version"], tag)
	}
	_ = w.Flush()
}
