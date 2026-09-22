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

	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/node"
	"barista/internal/output"
	"barista/internal/runner"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Health-check the barista environment",
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
	cmd.Flags().Bool("deep", false, "also re-probe JDKs (spawns java) and Node.js installations (spawns node), and verify repo origins against the manifest")
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
	add(checkUv())
	add(checkJavaHome())
	add(checkUserConfig())

	var jdkReg *jdk.Registry
	if p, err := jdk.RegistryPath(); err != nil {
		add(failed("jdk.json", "user", "jdkRegistryParse", output.CodeConfigError, err.Error(), "check that the user home directory is resolvable"))
	} else if reg, e := jdk.Load(p); e != nil {
		add(errResult("jdk.json", "user", "jdkRegistryParse", withHint(e, "fix or delete "+filepath.ToSlash(p))))
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
		add(failed("maven.json", "user", "mavenRegistryParse", output.CodeConfigError, err.Error(), "check that the user home directory is resolvable"))
	} else if reg, e := maven.Load(p); e != nil {
		add(errResult("maven.json", "user", "mavenRegistryParse", withHint(e, "fix or delete "+filepath.ToSlash(p))))
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

	var gradleReg *gradle.Registry
	if p, err := gradle.RegistryPath(); err != nil {
		add(failed("gradle.json", "user", "gradleRegistryParse", output.CodeConfigError, err.Error(), "check that the user home directory is resolvable"))
	} else if reg, e := gradle.Load(p); e != nil {
		add(errResult("gradle.json", "user", "gradleRegistryParse", withHint(e, "fix or delete "+filepath.ToSlash(p))))
	} else {
		gradleReg = reg
		add(ok("gradle.json", "user", "gradleRegistryParse", map[string]any{"registered": len(reg.Installations)}))
		for _, entry := range reg.Installations {
			entry := entry
			task(func(context.Context) output.Result { return checkGradleEntry(entry) })
		}
		add(checkGradleDefault(reg))
		if jdkReg != nil {
			add(checkGradleJdk(reg, jdkReg))
		} else {
			add(skipped("gradle.json", "user", "gradleJdk", "jdk.json unavailable"))
		}
		add(checkInstallDir("gradle.json installDir", "gradleInstallDir", reg.InstallDir, "barista gradle set-install-dir --reset"))
	}

	var nodeReg *node.Registry
	if p, err := node.RegistryPath(); err != nil {
		add(failed("node.json", "user", "nodeRegistryParse", output.CodeConfigError, err.Error(), "check that the user home directory is resolvable"))
	} else if reg, e := node.Load(p); e != nil {
		add(errResult("node.json", "user", "nodeRegistryParse", withHint(e, "fix or delete "+filepath.ToSlash(p))))
	} else {
		nodeReg = reg
		add(ok("node.json", "user", "nodeRegistryParse", map[string]any{"registered": len(reg.Installations)}))
		for _, entry := range reg.Installations {
			entry := entry
			task(func(context.Context) output.Result { return checkNodeEntry(entry) })
			if deep {
				task(func(context.Context) output.Result { return checkNodeProbe(entry) })
			}
		}
		add(checkNodeDefault(reg))
		add(checkInstallDir("node.json installDir", "nodeInstallDir", reg.InstallDir, "barista node set-install-dir --reset"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		add(failed("workspace", "workspace", "workspaceDetect", output.CodeWorkspaceNotFound, err.Error(), "the current directory may have been removed; cd into an existing directory and retry"))
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
		add(errResult(".barista", "workspace", "workspaceLoad", withHint(e, "fix the invalid files under .barista/")))
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
		for _, repo := range ws.Repos.Repos {
			add(checkJavaVersionFile(ws, repo, jdkReg))
		}
	}
	if mavenReg != nil {
		if v, has := ws.Props.String("maven.default"); has && v != "" {
			add(checkMavenDefaultSpec("workspace", "properties.json", v, "workspace properties", mavenReg))
		}
		for _, repo := range ws.Repos.Repos {
			add(checkMavenWrapperFile(ws, repo, mavenReg))
		}
	}
	if gradleReg != nil {
		if v, has := ws.Props.String("gradle.default"); has && v != "" {
			add(checkGradleDefaultSpec("workspace", "properties.json", v, "workspace properties", gradleReg))
		}
		for _, repo := range ws.Repos.Repos {
			add(checkGradleWrapperFile(ws, repo, gradleReg))
		}
	}
	if nodeReg != nil {
		if v, has := ws.Props.String("node"); has && v != "" {
			add(checkNodeSpec("workspace", "properties.json", v, "workspace properties", nodeReg))
		}
		for _, repo := range ws.Repos.Repos {
			if v, has := repo.Property("node"); has && v != "" {
				add(checkNodeSpec("workspace", repo.Name, v, "repo properties", nodeReg))
			}
		}
	}
	if v, has := ws.Props.String("maven.launch"); has && v != "" {
		add(checkMavenLaunch("workspace", "properties.json", v, "workspace properties"))
	}
	for _, repo := range ws.Repos.Repos {
		if v, has := repo.Property("maven.launch"); has && v != "" {
			add(checkMavenLaunch("workspace", repo.Name, v, "repo properties"))
		}
	}
	add(checkRepoDeps(ws))
	add(checkWorkspaceFile(ws.Root, "maven", "settings.xml", "settingsFile", "-s"))
	add(checkWorkspaceFile(ws.Root, "maven", "settings-security.xml", "settingsSecurityFile", "-Dsettings.security"))
	add(checkWorkspaceFile(ws.Root, "gradle", "init.gradle", "gradleInitFile", "-I"))
	add(checkWorkspaceFile(ws.Root, "gradle", "init.gradle.kts", "gradleInitKtsFile", "-I"))
	return results, tasks
}

func withHint(e *output.ErrInfo, hint string) *output.ErrInfo {
	if e.Hint == "" {
		e.Hint = hint
	}
	return e
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
	return errResult(name, "user", check, withHint(e, "fix or delete the user config file (~/.barista/config.json)"))
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
	p := output.NewPalette(color, cfg.ColorProfile)
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
