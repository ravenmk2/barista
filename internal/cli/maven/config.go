package mavencli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/maven"
	"barista/internal/output"
)

func configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the effective maven configuration (default, jdk, installDir) with its source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			cfg, p, ok := userSettings(cmd)
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
			detail := map[string]any{}

			if eff.Default == "" {
				detail["default"] = map[string]any{"value": "", "source": "none"}
			} else {
				d := map[string]any{"value": eff.Default, "source": eff.DefaultSource}
				if e := reg.Find(eff.Default); e != nil {
					d["resolved"] = map[string]any{"version": e.Version, "path": filepath.ToSlash(e.Path)}
				} else {
					d["error"] = map[string]any{
						"code":    output.CodeMavenNotFound,
						"message": fmt.Sprintf("default maven %q is not registered", eff.Default),
					}
				}
				detail["default"] = d
			}

			if eff.Jdk == "" {
				detail["jdk"] = map[string]any{"value": "", "source": "ambient"}
			} else {
				j := map[string]any{"value": eff.Jdk, "source": eff.JdkSource}
				entry, e := resolveJdkSpec(cmd, eff.Jdk)
				if e != nil {
					j["error"] = map[string]any{"code": e.Code, "message": e.Message}
				} else {
					j["resolved"] = map[string]any{
						"name":    entry.Name,
						"version": entry.Version,
						"path":    filepath.ToSlash(entry.Path),
					}
				}
				detail["jdk"] = j
			}

			installDir, err := maven.InstallDir(cfg.MavenInstallDir)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			installSrc := "builtin"
			if cfg.MavenInstallDir != "" {
				installSrc = "user"
			}
			detail["installDir"] = map[string]any{"value": filepath.ToSlash(installDir), "source": installSrc}

			res := output.Result{
				Name:   "config",
				Status: output.StatusOK,
				Action: "config",
				Detail: detail,
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				printConfig(detail, p)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}

func printConfig(detail map[string]any, p output.Palette) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SETTING\tVALUE\tSOURCE")

	def := detail["default"].(map[string]any)
	defValue := def["value"].(string)
	if defValue == "" {
		defValue = p.Dim("(none)")
	} else if r, ok := def["resolved"].(map[string]any); ok {
		defValue = fmt.Sprintf("%s (%v)", p.Cyan(defValue), r["version"])
	}
	defValue += errorSuffix(def, p)
	_, _ = fmt.Fprintf(w, "default\t%s\t%s\n", defValue, sourceLabel(def, "maven.json"))

	jdkD := detail["jdk"].(map[string]any)
	jdkValue := jdkD["value"].(string)
	if jdkValue == "" {
		jdkValue = p.Dim("(ambient, JAVA_HOME/PATH left untouched)")
	} else if r, ok := jdkD["resolved"].(map[string]any); ok {
		jdkValue = fmt.Sprintf("%s → %v %v", p.Cyan(jdkValue), r["name"], r["version"])
	}
	jdkValue += errorSuffix(jdkD, p)
	_, _ = fmt.Fprintf(w, "jdk\t%s\t%s\n", jdkValue, sourceLabel(jdkD, "maven.json"))

	inst := detail["installDir"].(map[string]any)
	_, _ = fmt.Fprintf(w, "installDir\t%v\t%s\n", inst["value"], sourceLabel(inst, "config.json"))
	_ = w.Flush()
}

func errorSuffix(d map[string]any, p output.Palette) string {
	if e, ok := d["error"].(map[string]any); ok {
		return p.Red(fmt.Sprintf("  error: %v: %v", e["code"], e["message"]))
	}
	return ""
}

func sourceLabel(d map[string]any, userFile string) string {
	switch d["source"] {
	case "user":
		return "user (~/.barista/" + userFile + ")"
	case "workspace":
		return "workspace (.barista/config.json)"
	case "builtin":
		return "builtin"
	default:
		return "-"
	}
}
