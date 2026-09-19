package jdkcli

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
)

func availableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "available",
		Short: "List JDK releases available for download from supported distros",
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
			var results []output.Result
			for _, distro := range jdk.SupportedDistros() {
				prov, _ := jdk.ProviderFor(distro)
				releases, e := prov.Available(cmd.Context())
				if e != nil {
					failResult(cmd, p, output.Result{Name: distro, Action: "available", Status: output.StatusFailed, Error: e})
					return nil
				}
				for _, r := range releases {
					name := distro + strconv.Itoa(r.Major)
					_, platErr := prov.ArchiveURL(r.Major, runtime.GOOS, runtime.GOARCH)
					results = append(results, output.Result{
						Name:   name,
						Status: output.StatusOK,
						Action: "available",
						Detail: map[string]any{
							"major":             r.Major,
							"version":           r.Version,
							"lts":               r.LTS,
							"latest":            r.Latest,
							"installed":         reg.Find(name) != nil,
							"platformSupported": platErr == nil,
						},
					})
				}
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printAvailable(results, p)
			}
			finish(cmd, results)
			return nil
		},
	}
}

func printAvailable(results []output.Result, p output.Palette) {
	if len(results) == 0 {
		fmt.Println(p.Dim("no releases available"))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tMAJOR\tVERSION\tTAGS")
	for _, res := range results {
		d := res.Detail
		var tags []string
		if d["lts"] == true {
			tags = append(tags, "lts")
		}
		if d["latest"] == true {
			tags = append(tags, "latest")
		}
		if d["installed"] == true {
			tags = append(tags, "installed")
		}
		if d["platformSupported"] == false {
			tags = append(tags, "unsupported-platform")
		}
		tag := strings.Join(tags, ",")
		if tag == "" {
			tag = "-"
		} else {
			tag = p.Yellow(tag)
		}
		version, _ := d["version"].(string)
		if version == "" {
			version = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%v\t%s\t%s\n", res.Name, d["major"], version, tag)
	}
	_ = w.Flush()
}
