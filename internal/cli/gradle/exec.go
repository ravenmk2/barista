package gradlecli

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

	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

func runExec(cmd *cobra.Command, args []string) error {
	*exitCode = 0
	atDash := cmd.ArgsLenAtDash()
	if atDash != 0 && len(args) > 0 {
		fail(cmd, &output.ErrInfo{
			Code:    output.CodeUsageError,
			Message: "gradle tasks and flags must follow \"--\"",
			Hint:    "example: barista gradle -- build test",
		})
		return nil
	}
	if atDash == -1 && len(args) == 0 && !cmd.Flags().Changed("jdk") && !cmd.Flags().Changed("dry-run") {
		return cmd.Help()
	}
	jdkFlag, _ := cmd.Flags().GetString("jdk")
	_, p, ok := userSettings(cmd)
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
		in.props = ws.Props
	}
	gradlePath, err := gradle.RegistryPath()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	gradleReg, e := gradle.Load(gradlePath)
	if e != nil {
		return nil, e
	}
	in.gradleReg = gradleReg
	jdkPath, err := jdk.RegistryPath()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	jdkReg, e := jdk.Load(jdkPath)
	if e != nil {
		return nil, e
	}
	in.jdkReg = jdkReg
	if detectFilesEnabled(in.props) {
		if major, distro, file := detectJavaVersionFile(cwd, in.wsRoot); major > 0 {
			in.javaVersionMajor, in.javaVersionDistro, in.javaVersionFile = major, distro, file
		}
		if v, f, ok := wrapperVersion(cwd, in.wsRoot); ok {
			in.wrapperVersion, in.wrapperFile = v, f
		}
	}
	return planExec(in)
}

func detectFilesEnabled(props workspace.Properties) bool {
	if v, ok := props.Bool("detect.files"); ok {
		return v
	}
	return true
}

func detectJavaVersionFile(cwd, wsRoot string) (int, string, string) {
	boundary := wsRoot
	if gitRoot, ok := workspace.FindGitRoot(cwd, wsRoot); ok {
		boundary = gitRoot
	} else if wsRoot == "" {
		boundary = cwd
	}
	path, ok := workspace.FindUpward(cwd, boundary, ".java-version")
	if !ok {
		return 0, "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", ""
	}
	major, distro, ok := jdk.ParseJavaVersionFile(string(data))
	if !ok {
		return 0, "", ""
	}
	return major, distro, path
}

func runPlan(cmd *cobra.Command, plan *execPlan) {
	ls := plan.launch
	var c *exec.Cmd
	if ls.viaCmd {
		args := append([]string{"/c", ls.bin}, ls.args...)
		c = exec.Command("cmd", args...)
	} else {
		c = exec.Command(ls.bin, ls.args...)
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
		Code:    output.CodeGradleExecFailed,
		Message: fmt.Sprintf("cannot start %s: %v", filepath.ToSlash(ls.bin), err),
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
		gradleDetail := map[string]any{
			"name":    plan.gradleName,
			"version": plan.gradleVersion,
			"home":    filepath.ToSlash(plan.gradleHome),
			"bin":     filepath.ToSlash(plan.gradleBin),
			"source":  plan.gradleSrc,
		}
		if plan.gradleFile != "" {
			gradleDetail["file"] = filepath.ToSlash(plan.gradleFile)
		}
		detail := map[string]any{
			"gradle":  gradleDetail,
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
			jdkDetail := map[string]any{
				"spec":     plan.jdkSpec,
				"name":     plan.jdkName,
				"version":  plan.jdkVersion,
				"javaHome": filepath.ToSlash(plan.javaHome),
				"source":   plan.jdkSrc,
			}
			if plan.jdkFile != "" {
				jdkDetail["file"] = filepath.ToSlash(plan.jdkFile)
			}
			detail["jdk"] = jdkDetail
		} else {
			detail["jdk"] = map[string]any{"source": "ambient"}
		}
		if len(plan.initScripts) > 0 {
			scripts := make([]string, len(plan.initScripts))
			for i, s := range plan.initScripts {
				scripts[i] = filepath.ToSlash(s)
			}
			detail["initScripts"] = scripts
		}
		if plan.gradleUserHome != "" {
			detail["gradleUserHome"] = filepath.ToSlash(plan.gradleUserHome)
		}
		name := plan.repoName
		if name == "" {
			name = "gradle"
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
	gradleSrc := plan.gradleSrc
	if plan.gradleFile != "" {
		gradleSrc = fmt.Sprintf("%s, %s", gradleSrc, filepath.ToSlash(plan.gradleFile))
	}
	_, _ = fmt.Fprintf(w, "gradle\t%s %s [%s]\n", p.Cyan(plan.gradleName), plan.gradleVersion, gradleSrc)
	_, _ = fmt.Fprintf(w, "bin\t%s\n", filepath.ToSlash(plan.launch.bin))
	if plan.javaHome != "" {
		jdkSrc := plan.jdkSrc
		if plan.jdkFile != "" {
			jdkSrc = fmt.Sprintf("%s, %s", jdkSrc, filepath.ToSlash(plan.jdkFile))
		}
		_, _ = fmt.Fprintf(w, "jdk\t%s → %s %s [%s]\n", p.Cyan(plan.jdkSpec), plan.jdkName, plan.jdkVersion, jdkSrc)
		_, _ = fmt.Fprintf(w, "JAVA_HOME\t%s\n", filepath.ToSlash(plan.javaHome))
	} else {
		_, _ = fmt.Fprintf(w, "jdk\t%s\n", p.Dim("(ambient, JAVA_HOME/PATH left untouched)"))
	}
	for _, s := range plan.initScripts {
		_, _ = fmt.Fprintf(w, "init\t-I %s\n", filepath.ToSlash(s))
	}
	if plan.gradleUserHome != "" {
		_, _ = fmt.Fprintf(w, "gradle.user.home\t--gradle-user-home %s\n", filepath.ToSlash(plan.gradleUserHome))
	}
	_, _ = fmt.Fprintf(w, "command\t%s\n", commandLine(plan))
	_ = w.Flush()
}

func commandLine(plan *execPlan) string {
	ls := plan.launch
	var parts []string
	if ls.viaCmd {
		parts = append(parts, "cmd", "/c")
	}
	parts = append(parts, ls.bin)
	for _, a := range ls.args {
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
