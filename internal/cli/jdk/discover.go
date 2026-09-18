package jdkcli

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/runner"
)

type discoverOutcome struct {
	cand jdk.Candidate
	info jdk.Info
	err  *output.ErrInfo
}

func discoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Discover installed JDKs (JAVA_HOME envs, sdkman, system locations, PATH) and register them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			cfg, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			cands := jdk.DiscoverCandidates()
			if len(cands) == 0 {
				if !jsonOut {
					fmt.Println("no JDK candidates found")
				}
				finish(cmd, []output.Result{})
				return nil
			}
			parallel, _ := cmd.Flags().GetInt("parallel")
			if !cmd.Flags().Changed("parallel") && cfg.Parallel > 0 {
				parallel = cfg.Parallel
			}
			tasks := make([]runner.Task[discoverOutcome], len(cands))
			for i, c := range cands {
				c := c
				tasks[i] = runner.Task[discoverOutcome]{
					Name: c.Path,
					Run: func(context.Context) discoverOutcome {
						info, e := jdk.Probe(c.Path)
						return discoverOutcome{cand: c, info: info, err: e}
					},
				}
			}
			outcomes := runner.Run(cmd.Context(), tasks, parallel, nil)

			results := make([]output.Result, 0, len(outcomes))
			registered, skipped, failed := 0, 0, 0
			for _, o := range outcomes {
				res := output.Result{
					Name:   filepath.ToSlash(o.cand.Path),
					Path:   filepath.ToSlash(o.cand.Path),
					Action: "discover",
				}
				switch {
				case o.err != nil:
					res.Status = output.StatusSkipped
					res.Error = o.err
					skipped++
				default:
					if name := registeredAs(reg, o.info.Home); name != "" {
						res.Status = output.StatusSkipped
						res.Detail = map[string]any{"reason": "alreadyRegistered", "as": name}
						skipped++
						break
					}
					name := reg.AvailableName(o.info.Distro + strconv.Itoa(o.info.Major))
					if e := reg.Add(jdk.Entry{Name: name, Major: o.info.Major, Version: o.info.Version, Path: o.info.Home}); e != nil {
						res.Status = output.StatusFailed
						res.Error = e
						failed++
						break
					}
					registered++
					res.Name = name
					res.Path = filepath.ToSlash(o.info.Home)
					res.Status = output.StatusOK
					res.Detail = map[string]any{
						"major":   o.info.Major,
						"version": o.info.Version,
						"distro":  o.info.Distro,
						"source":  o.cand.Source,
					}
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
						fmt.Printf("%s %s (%s %v) at %s [%v]\n", p.Green("registered"), p.Cyan(res.Name), res.Detail["distro"], res.Detail["version"], res.Path, res.Detail["source"])
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
				if len(results) > 0 && registered == len(results) {
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

func registeredAs(reg *jdk.Registry, home string) string {
	for _, e := range reg.JDKs {
		if jdk.SamePath(e.Path, home) {
			return e.Name
		}
	}
	return ""
}
