package mavencli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

func configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the effective maven configuration (default, jdk, installDir) with its source",
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
			wsCfg, ok := workspaceConfig(cmd)
			if !ok {
				return nil
			}
			eff := effective(reg, wsCfg)
			detail := map[string]any{}

			jdkSpec, jdkSrc := eff.Jdk, eff.JdkSource
			if wsRoot := findWorkspaceRoot(); wsRoot != "" {
				cwd, err := os.Getwd()
				if err != nil {
					fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
					return nil
				}
				ws, err := workspace.Load(cwd)
				if err != nil {
					e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
					var le *workspace.LoadError
					if errors.As(err, &le) {
						e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
					}
					fail(cmd, e)
					return nil
				}
				if repo := ws.MatchRepo(cwd); repo != nil {
					detail["repo"] = map[string]any{"name": repo.Name, "path": filepath.ToSlash(repo.Path)}
					if v, ok := repo.Property("jdk"); ok && v != "" {
						jdkSpec, jdkSrc = v, "repo"
					}
				} else {
					detail["repo"] = map[string]any{"name": "", "source": "none"}
				}
				s := filepath.Join(wsRoot, ".barista", "maven", "settings.xml")
				if _, err := os.Stat(s); err == nil {
					detail["settings"] = map[string]any{"value": filepath.ToSlash(s), "source": "workspace"}
				} else {
					detail["settings"] = map[string]any{"value": "", "source": "ambient"}
				}
				if v, ok := wsCfg.Property("maven.repo.local"); ok && v != "" {
					v = workspace.ExpandHome(v)
					if !filepath.IsAbs(v) {
						v = filepath.Join(wsRoot, v)
					}
					detail["repoLocal"] = map[string]any{"value": filepath.ToSlash(v), "source": "workspace"}
				} else {
					detail["repoLocal"] = map[string]any{"value": "", "source": "none"}
				}
			}

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

			if jdkSpec == "" {
				detail["jdk"] = map[string]any{"value": "", "source": "ambient"}
			} else {
				j := map[string]any{"value": jdkSpec, "source": jdkSrc}
				entry, e := resolveJdkSpec(cmd, jdkSpec)
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

			installDir, err := maven.InstallDir(reg.InstallDir)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			installSrc := "builtin"
			if reg.InstallDir != "" {
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

	if rd, ok := detail["repo"].(map[string]any); ok {
		name, _ := rd["name"].(string)
		src := "-"
		if name != "" {
			name = fmt.Sprintf("%s (%v)", p.Cyan(name), rd["path"])
			src = "repos.json"
		} else {
			name = p.Dim("(no repo matches cwd)")
		}
		_, _ = fmt.Fprintf(w, "repo\t%s\t%s\n", name, src)
	}
	if sd, ok := detail["settings"].(map[string]any); ok {
		v, _ := sd["value"].(string)
		src := "-"
		if v != "" {
			src = "workspace (.barista/maven/settings.xml)"
		} else {
			v = p.Dim("(none, Maven default applies)")
		}
		_, _ = fmt.Fprintf(w, "settings\t%s\t%s\n", v, src)
	}
	if ld, ok := detail["repoLocal"].(map[string]any); ok {
		v, _ := ld["value"].(string)
		if v == "" {
			v = p.Dim("(none)")
		}
		_, _ = fmt.Fprintf(w, "repo.local\t%s\t%s\n", v, sourceLabel(ld, "config.json"))
	}

	inst := detail["installDir"].(map[string]any)
	_, _ = fmt.Fprintf(w, "installDir\t%v\t%s\n", inst["value"], sourceLabel(inst, "maven.json"))
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
	case "repo":
		return "repo (repos.json properties)"
	case "builtin":
		return "builtin"
	default:
		return "-"
	}
}
