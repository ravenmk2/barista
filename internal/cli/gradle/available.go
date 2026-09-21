package gradlecli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/gradle"
	"barista/internal/output"
	"barista/internal/toolversion"
)

func availableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "available",
		Short: "List Gradle versions available for download from services.gradle.org",
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
			versions, e := gradle.Available(cmd.Context())
			if e != nil {
				failResult(cmd, p, output.Result{Name: "services.gradle.org", Action: "available", Status: output.StatusFailed, Error: e})
				return nil
			}
			if all, _ := cmd.Flags().GetBool("all"); !all {
				var finals []gradle.AvailableVersion
				for _, v := range versions {
					if !v.Prerelease {
						finals = append(finals, v)
					}
				}
				versions = gradle.RecentMajors(gradle.LatestPerMinor(finals), 2)
			}
			results := make([]output.Result, 0, len(versions))
			for _, v := range versions {
				installed := false
				for _, inst := range reg.Installations {
					if toolversion.Compare(inst.Version, v.Version) == 0 {
						installed = true
						break
					}
				}
				detail := map[string]any{
					"version":   v.Version,
					"latest":    v.Latest,
					"installed": installed,
				}
				if v.Prerelease {
					detail["prerelease"] = true
				}
				results = append(results, output.Result{
					Name:   gradle.NameFor(v.Version),
					Status: output.StatusOK,
					Action: "available",
					Detail: detail,
				})
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printAvailable(results, p)
			}
			finish(cmd, results)
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "list every version, including rc/milestone prereleases (default: latest final of each minor line in the two newest majors)")
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
		if d["prerelease"] == true {
			tags = append(tags, qualifierTag(fmt.Sprintf("%v", d["version"])))
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

func qualifierTag(version string) string {
	i := strings.IndexByte(version, '-')
	if i < 0 {
		return "prerelease"
	}
	q := version[i+1:]
	j := 0
	for j < len(q) && (q[j] >= 'a' && q[j] <= 'z' || q[j] >= 'A' && q[j] <= 'Z') {
		j++
	}
	if j == 0 {
		return "prerelease"
	}
	return q[:j]
}
