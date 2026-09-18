package mavencli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
)

func discoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Discover installed Mavens (MAVEN_HOME/M2_HOME envs, sdkman, brew, scoop, system locations, PATH) and register them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			cands := maven.DiscoverCandidates()
			if len(cands) == 0 {
				if !jsonOut {
					fmt.Println("no Maven candidates found")
				}
				finish(cmd, []output.Result{})
				return nil
			}
			results := make([]output.Result, 0, len(cands))
			registered, skipped, failed := 0, 0, 0
			for _, c := range cands {
				res := output.Result{
					Name:   filepath.ToSlash(c.Path),
					Path:   filepath.ToSlash(c.Path),
					Action: "discover",
				}
				info, e := maven.Probe(c.Path)
				if e != nil {
					res.Status = output.StatusSkipped
					res.Error = e
					skipped++
					results = append(results, res)
					continue
				}
				if name := registeredAs(reg, info.Home); name != "" {
					res.Status = output.StatusSkipped
					res.Detail = map[string]any{"reason": "alreadyRegistered", "as": name}
					skipped++
					results = append(results, res)
					continue
				}
				name := autoName(reg, info.Version)
				if e := reg.Add(maven.Entry{Name: name, Version: info.Version, Path: info.Home}); e != nil {
					res.Status = output.StatusFailed
					res.Error = e
					failed++
					results = append(results, res)
					continue
				}
				registered++
				res.Name = name
				res.Path = filepath.ToSlash(info.Home)
				res.Status = output.StatusOK
				res.Detail = map[string]any{
					"version": info.Version,
					"source":  c.Source,
				}
				results = append(results, res)
			}
			if registered > 0 && !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			if !jsonOut {
				for _, res := range results {
					switch {
					case res.Status == output.StatusOK:
						fmt.Printf("%s %s (%v) at %s [%v]\n", p.Green("registered"), p.Cyan(res.Name), res.Detail["version"], res.Path, res.Detail["source"])
					case res.Status == output.StatusFailed:
						fmt.Println(p.Red(fmt.Sprintf("%-10s %s (%s)", res.Status, res.Name, res.Error.Message)))
					case res.Error != nil:
						fmt.Println(p.Dim(fmt.Sprintf("%-10s %s (%s)", res.Status, res.Name, res.Error.Message)))
					default:
						fmt.Println(p.Dim(fmt.Sprintf("%-10s %s (already registered as %q)", res.Status, res.Name, res.Detail["as"])))
					}
				}
				seg := func(v int, label string, style func(string) string) string {
					t := fmt.Sprintf("%d %s", v, label)
					if v > 0 {
						return style(t)
					}
					return p.Dim(t)
				}
				var line string
				if registered == len(results) {
					line = p.Green(fmt.Sprintf("%d candidates: %d registered", len(results), registered)) +
						p.Dim(fmt.Sprintf(", %d skipped, %d failed", skipped, failed))
				} else {
					line = fmt.Sprintf("%d candidates: ", len(results)) +
						seg(registered, "registered", p.Green) + ", " +
						seg(skipped, "skipped", p.Yellow) + ", " +
						seg(failed, "failed", p.Red)
				}
				fmt.Println(line)
			}
			finish(cmd, results)
			return nil
		},
	}
}

func registeredAs(reg *maven.Registry, home string) string {
	for _, e := range reg.Installations {
		if jdk.SamePath(e.Path, home) {
			return e.Name
		}
	}
	return ""
}
