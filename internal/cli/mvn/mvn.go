package mvncli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/maven"
	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "mvn [--jdk spec] [--dry-run] -- <mvn args...>",
		Short: "Run Maven with the workspace-aware JDK, settings.xml and local repo",
		Long: "Run Maven in the current directory. Arguments after \"--\" are passed through verbatim.\n" +
			"Resolution (high to low): JDK: --jdk > repo properties[\"maven.jdk\"] > workspace maven.jdk > user maven.json jdk > ambient.\n" +
			"settings.xml: .barista/maven/settings.xml is injected as -s when present (skipped when you pass -s yourself).\n" +
			"maven.repo.local: workspace property injected as -Dmaven.repo.local (skipped when you pass it yourself).",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			atDash := cmd.ArgsLenAtDash()
			if atDash != 0 && len(args) > 0 {
				fail(cmd, &output.ErrInfo{
					Code:    output.CodeUsageError,
					Message: "maven goals and flags must follow \"--\"",
					Hint:    "example: barista mvn -- clean install -DskipTests",
				})
				return nil
			}
			jdkFlag, _ := cmd.Flags().GetString("jdk")
			p, ok := palette(cmd)
			if !ok {
				return nil
			}
			plan, e := buildPlan(cmd, jdkFlag, args)
			if e != nil {
				fail(cmd, e)
				return nil
			}
			if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
				printPlan(cmd, plan, p)
				return nil
			}
			runPlan(cmd, plan)
			return nil
		},
	}
	cmd.Flags().String("jdk", "", "JDK spec (registry name or major version) used to run Maven; overrides every config level")
	cmd.Flags().Bool("dry-run", false, "print the resolved environment and full command line without executing")
	return cmd
}

func buildPlan(cmd *cobra.Command, jdkFlag string, passthrough []string) (*execPlan, *output.ErrInfo) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	in := planInput{
		cwd:         cwd,
		goos:        runtime.GOOS,
		jdkFlag:     jdkFlag,
		passthrough: passthrough,
	}
	if root := workspace.FindWorkspaceRoot(cwd); root != "" {
		ws, err := workspace.Load(cwd)
		if err != nil {
			e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
			var le *workspace.LoadError
			if errors.As(err, &le) {
				e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
			}
			return nil, e
		}
		in.wsRoot = ws.Root
		in.repos = ws.Repos.Repos
		in.wsCfg = ws.Cfg
	}
	mavenPath, err := maven.RegistryPath()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	mavenReg, e := maven.Load(mavenPath)
	if e != nil {
		return nil, e
	}
	in.mavenReg = mavenReg
	jdkPath, err := jdk.RegistryPath()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	jdkReg, e := jdk.Load(jdkPath)
	if e != nil {
		return nil, e
	}
	in.jdkReg = jdkReg
	return planExec(in)
}

func runPlan(cmd *cobra.Command, plan *execPlan) {
	var c *exec.Cmd
	if plan.goos == "windows" {
		args := append([]string{"/c", plan.mavenBin}, plan.args...)
		c = exec.Command("cmd", args...)
	} else {
		c = exec.Command(plan.mavenBin, plan.args...)
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = childEnv(plan.javaHome)
	err := c.Run()
	if err == nil {
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		*exitCode = 1
		return
	}
	fail(cmd, &output.ErrInfo{
		Code:    output.CodeMavenExecFailed,
		Message: fmt.Sprintf("cannot start %s: %v", filepath.ToSlash(plan.mavenBin), err),
	})
}

func childEnv(javaHome string) []string {
	env := os.Environ()
	if javaHome == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if strings.EqualFold(k, "JAVA_HOME") {
			if !replaced {
				out = append(out, "JAVA_HOME="+javaHome)
				replaced = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, "JAVA_HOME="+javaHome)
	}
	return out
}

func printPlan(cmd *cobra.Command, plan *execPlan, p output.Palette) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		detail := map[string]any{
			"maven": map[string]any{
				"name":    plan.mavenName,
				"version": plan.mavenVersion,
				"home":    filepath.ToSlash(plan.mavenHome),
				"bin":     filepath.ToSlash(plan.mavenBin),
				"source":  plan.mavenSrc,
			},
			"args":    plan.args,
			"command": commandLine(plan),
		}
		if plan.wsRoot != "" {
			detail["workspace"] = filepath.ToSlash(plan.wsRoot)
		}
		if plan.repoName != "" {
			detail["repo"] = map[string]any{"name": plan.repoName, "path": filepath.ToSlash(plan.repoPath)}
		}
		if plan.javaHome != "" {
			detail["jdk"] = map[string]any{
				"spec":     plan.jdkSpec,
				"name":     plan.jdkName,
				"version":  plan.jdkVersion,
				"javaHome": filepath.ToSlash(plan.javaHome),
				"source":   plan.jdkSrc,
			}
		} else {
			detail["jdk"] = map[string]any{"source": "ambient"}
		}
		if plan.settings != "" {
			detail["settings"] = filepath.ToSlash(plan.settings)
		}
		if plan.settingsSecurity != "" {
			detail["settingsSecurity"] = filepath.ToSlash(plan.settingsSecurity)
		}
		if plan.repoLocal != "" {
			detail["repoLocal"] = filepath.ToSlash(plan.repoLocal)
		}
		name := plan.repoName
		if name == "" {
			name = "mvn"
		}
		res := output.Result{Name: name, Status: output.StatusOK, Action: "dry-run", Detail: detail}
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), filepath.ToSlash(plan.wsRoot), []output.Result{res}))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if plan.wsRoot != "" {
		_, _ = fmt.Fprintf(w, "workspace\t%s\n", filepath.ToSlash(plan.wsRoot))
	} else {
		_, _ = fmt.Fprintf(w, "workspace\t%s\n", p.Dim("(none)"))
	}
	if plan.repoName != "" {
		_, _ = fmt.Fprintf(w, "repo\t%s (%s)\n", p.Cyan(plan.repoName), filepath.ToSlash(plan.repoPath))
	} else {
		_, _ = fmt.Fprintf(w, "repo\t%s\n", p.Dim("(no repo matches cwd)"))
	}
	_, _ = fmt.Fprintf(w, "maven\t%s %s [%s]\n", p.Cyan(plan.mavenName), plan.mavenVersion, plan.mavenSrc)
	_, _ = fmt.Fprintf(w, "bin\t%s\n", filepath.ToSlash(plan.mavenBin))
	if plan.javaHome != "" {
		_, _ = fmt.Fprintf(w, "jdk\t%s → %s %s [%s]\n", p.Cyan(plan.jdkSpec), plan.jdkName, plan.jdkVersion, plan.jdkSrc)
		_, _ = fmt.Fprintf(w, "JAVA_HOME\t%s\n", filepath.ToSlash(plan.javaHome))
	} else {
		_, _ = fmt.Fprintf(w, "jdk\t%s\n", p.Dim("(ambient, JAVA_HOME/PATH left untouched)"))
	}
	if plan.settings != "" {
		_, _ = fmt.Fprintf(w, "settings\t-s %s\n", filepath.ToSlash(plan.settings))
	}
	if plan.settingsSecurity != "" {
		_, _ = fmt.Fprintf(w, "settings-security\t-Dsettings.security=%s\n", filepath.ToSlash(plan.settingsSecurity))
	}
	if plan.repoLocal != "" {
		_, _ = fmt.Fprintf(w, "repo.local\t-Dmaven.repo.local=%s\n", filepath.ToSlash(plan.repoLocal))
	}
	_, _ = fmt.Fprintf(w, "command\t%s\n", commandLine(plan))
	_ = w.Flush()
}

func commandLine(plan *execPlan) string {
	parts := make([]string, 0, len(plan.args)+3)
	if plan.goos == "windows" {
		parts = append(parts, "cmd", "/c")
	}
	parts = append(parts, plan.mavenBin)
	for _, a := range plan.args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(a string) string {
	if strings.ContainsAny(a, " \t\"") {
		return `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
	}
	return a
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
}

func fail(cmd *cobra.Command, e *output.ErrInfo) {
	fmt.Fprintf(os.Stderr, "barista: %s: %s\n", e.Code, e.Message)
	if e.Hint != "" {
		fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
	}
	*exitCode = 2
}

func palette(cmd *cobra.Command) (output.Palette, bool) {
	cfg, err := workspace.LoadUserConfig()
	if err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		fail(cmd, e)
		return output.NewPalette(false), false
	}
	return output.NewPalette(output.ColorEnabled(cfg.Color)), true
}
