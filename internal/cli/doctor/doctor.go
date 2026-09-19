package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/runner"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Health-check the barista environment (user level, plus workspace level when inside one)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			deep, _ := cmd.Flags().GetBool("deep")
			results, tasks := assemble(cmd, deep)
			if len(tasks) > 0 {
				parallel, _ := cmd.Flags().GetInt("parallel")
				if parallel < 1 {
					parallel = 1
				}
				results = append(results, runner.Run(cmd.Context(), tasks, parallel, nil)...)
			}
			render(cmd, results)
			*exitCode = output.ExitCode(results)
			return nil
		},
	}
	cmd.Flags().Bool("deep", false, "also re-probe JDKs (spawns java) and verify repo origins against the manifest")
	return cmd
}

func assemble(cmd *cobra.Command, deep bool) ([]output.Result, []runner.Task[output.Result]) {
	var results []output.Result
	var tasks []runner.Task[output.Result]
	add := func(r output.Result) { results = append(results, r) }
	task := func(fn func(ctx context.Context) output.Result) {
		tasks = append(tasks, runner.Task[output.Result]{Run: func(ctx context.Context) output.Result { return fn(ctx) }})
	}
	ctx := cmd.Context()

	add(checkGitOnPath())
	add(checkJavaHome())
	add(checkUserConfig())

	var jdkReg *jdk.Registry
	if p, err := jdk.RegistryPath(); err != nil {
		add(failed("jdk.json", "user", "jdkRegistryParse", output.CodeConfigError, err.Error(), ""))
	} else if reg, e := jdk.Load(p); e != nil {
		add(errResult("jdk.json", "user", "jdkRegistryParse", e))
	} else {
		jdkReg = reg
		add(ok("jdk.json", "user", "jdkRegistryParse", map[string]any{"registered": len(reg.JDKs)}))
		for _, entry := range reg.JDKs {
			entry := entry
			task(func(context.Context) output.Result { return checkJdkEntry(entry) })
			if deep {
				task(func(context.Context) output.Result { return checkJdkProbe(entry) })
			}
		}
		add(checkJdkDefaults(reg))
		add(checkInstallDir("jdk.json installDir", "jdkInstallDir", reg.InstallDir, "barista jdk set-install-dir --reset"))
	}

	var mavenReg *maven.Registry
	if p, err := maven.RegistryPath(); err != nil {
		add(failed("maven.json", "user", "mavenRegistryParse", output.CodeConfigError, err.Error(), ""))
	} else if reg, e := maven.Load(p); e != nil {
		add(errResult("maven.json", "user", "mavenRegistryParse", e))
	} else {
		mavenReg = reg
		add(ok("maven.json", "user", "mavenRegistryParse", map[string]any{"registered": len(reg.Installations)}))
		for _, entry := range reg.Installations {
			entry := entry
			task(func(context.Context) output.Result { return checkMavenEntry(entry) })
		}
		add(checkMavenDefault(reg))
		if jdkReg != nil {
			add(checkMavenJdk(reg, jdkReg))
		} else {
			add(skipped("maven.json", "user", "mavenJdk", "jdk.json unavailable"))
		}
		add(checkInstallDir("maven.json installDir", "mavenInstallDir", reg.InstallDir, "barista maven set-install-dir --reset"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		add(failed("workspace", "workspace", "workspaceDetect", output.CodeWorkspaceNotFound, err.Error(), ""))
		return results, tasks
	}
	root := workspace.FindWorkspaceRoot(cwd)
	if root == "" {
		add(skipped("workspace", "workspace", "workspaceDetect", "not inside a workspace (user level only)"))
		return results, tasks
	}
	ws, err := workspace.Load(cwd)
	if err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		add(errResult(".barista", "workspace", "workspaceLoad", e))
		return results, tasks
	}
	add(ok("workspace", "workspace", "workspaceLoad", map[string]any{
		"root": filepath.ToSlash(ws.Root), "repos": len(ws.Repos.Repos),
	}))
	for _, repo := range ws.Repos.Repos {
		repo := repo
		task(func(context.Context) output.Result { return checkRepoCheckout(ctx, ws, repo, deep) })
	}
	if jdkReg != nil {
		if v, has := ws.Props.String("jdk"); has && v != "" {
			add(checkJdkSpec("workspace", "properties.json", v, "workspace properties", jdkReg))
		}
		for _, repo := range ws.Repos.Repos {
			if v, has := repo.Property("jdk"); has && v != "" {
				add(checkJdkSpec("workspace", repo.Name, v, "repo properties", jdkReg))
			}
		}
	}
	if mavenReg != nil {
		if v, has := ws.Props.String("maven.default"); has && v != "" {
			add(checkMavenDefaultSpec("workspace", "properties.json", v, "workspace properties", mavenReg))
		}
	}
	if v, has := ws.Props.String("maven.startup"); has && v != "" {
		add(checkMavenStartup("workspace", "properties.json", v, "workspace properties"))
	}
	for _, repo := range ws.Repos.Repos {
		if v, has := repo.Property("maven.startup"); has && v != "" {
			add(checkMavenStartup("workspace", repo.Name, v, "repo properties"))
		}
	}
	add(checkSettingsFile(ws.Root, "settings.xml", "settingsFile", "-s"))
	add(checkSettingsFile(ws.Root, "settings-security.xml", "settingsSecurityFile", "-Dsettings.security"))
	return results, tasks
}

func checkUserConfig() output.Result {
	const name, check = "config.json", "userConfigParse"
	_, err := workspace.LoadUserConfig()
	if err == nil {
		return ok(name, "user", check, nil)
	}
	e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	var le *workspace.LoadError
	if errors.As(err, &le) {
		e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
	}
	return errResult(name, "user", check, e)
}

func render(cmd *cobra.Command, results []output.Result) {
	jsonOut, _ := cmd.Flags().GetBool("json")
	wsRoot := ""
	for _, r := range results {
		if r.Detail["check"] == "workspaceLoad" && r.Status == output.StatusOK {
			wsRoot, _ = r.Detail["root"].(string)
		}
	}
	if jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), wsRoot, results))
		return
	}
	cfg, err := workspace.LoadUserConfig()
	color := err == nil && output.ColorEnabled(cfg.Color)
	p := output.NewPalette(color)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, scope := range []string{"user", "workspace"} {
		first := true
		for _, r := range results {
			if r.Detail["scope"] != scope {
				continue
			}
			if first {
				_, _ = fmt.Fprintf(w, "%s\n", p.Cyan(scope))
				first = false
			}
			switch r.Status {
			case output.StatusOK:
				_, _ = fmt.Fprintf(w, "  %s\t%s\n", p.Green("ok"), r.Name)
			case output.StatusSkipped:
				reason, _ := r.Detail["reason"].(string)
				_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\n", p.Dim("skipped"), r.Name, p.Dim(reason))
			case output.StatusFailed:
				_, _ = fmt.Fprintf(w, "  %s\t%s\t%s: %s\n", p.Red("failed"), r.Name, r.Error.Code, r.Error.Message)
				if r.Error.Hint != "" {
					_, _ = fmt.Fprintf(w, "  \t\t%s\n", p.Dim("hint: "+r.Error.Hint))
				}
			}
		}
	}
	_ = w.Flush()
	s := output.Summarize(results)
	fmt.Printf("%d checks: %d ok, %d skipped, %d failed\n", s.Total, s.OK, s.Skipped, s.Failed)
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
}
