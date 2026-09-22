package nodecli

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

	"barista/internal/node"
	"barista/internal/output"
	"barista/internal/workspace"
)

func runExec(cmd *cobra.Command, args []string) error {
	runTool(cmd, "node", args)
	return nil
}

// RunToolExec implements the shared node/npm/npx executor contract for the
// npm and npx root commands (package npmcli builds the cobra shells; the node
// group root calls runExec).
func RunToolExec(cmd *cobra.Command, exit *int, tool string, args []string) {
	exitCode = exit
	runTool(cmd, tool, args)
}

func runTool(cmd *cobra.Command, tool string, args []string) {
	*exitCode = 0
	atDash := cmd.ArgsLenAtDash()
	if atDash != 0 && len(args) > 0 {
		fail(cmd, &output.ErrInfo{
			Code:    output.CodeUsageError,
			Message: tool + " arguments must follow \"--\"",
			Hint:    "example: barista " + tool + " -- " + toolUsageHint(tool),
		})
		return
	}
	if tool == "node" && atDash == -1 && len(args) == 0 && !cmd.Flags().Changed("node") && !cmd.Flags().Changed("dry-run") {
		_ = cmd.Help()
		return
	}
	nodeFlag, _ := cmd.Flags().GetString("node")
	_, p, ok := userSettings(cmd)
	if !ok {
		return
	}
	plan, e := buildPlan(cmd, tool, nodeFlag, args)
	if e != nil {
		fail(cmd, e)
		return
	}
	if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
		printPlan(cmd, plan, p)
		return
	}
	runPlan(cmd, plan)
}

func toolUsageHint(tool string) string {
	switch tool {
	case "node":
		return "-e \"console.log(1)\""
	case "npm":
		return "install"
	default:
		return "vite --version"
	}
}

func buildPlan(cmd *cobra.Command, tool, nodeFlag string, passthrough []string) (*execPlan, *output.ErrInfo) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	in := planInput{
		cwd:         cwd,
		goos:        runtime.GOOS,
		tool:        tool,
		nodeFlag:    nodeFlag,
		passthrough: passthrough,
	}
	if root := workspace.FindWorkspaceRoot(cwd); root != "" {
		ws, err := workspace.Load(cwd)
		if err != nil {
			return nil, configErrInfo(err)
		}
		in.wsRoot = ws.Root
		in.repos = ws.Repos.Repos
		in.props = ws.Props
	}
	regPath, err := node.RegistryPath()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	reg, e := node.Load(regPath)
	if e != nil {
		return nil, e
	}
	in.nodeReg = reg
	if bin, err := exec.LookPath(tool); err == nil {
		in.ambientBin = bin
	}
	if detectFilesEnabled(in.props) {
		if v, f, ok := detectNodeVersionFile(cwd, in.wsRoot); ok {
			in.fileVersion, in.filePath = v, f
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

// detectNodeVersionFile finds the nearest .node-version (wins) or .nvmrc
// between cwd and the enclosing checkout root (or workspace root when not in
// a checkout) and parses the declared version.
func detectNodeVersionFile(cwd, wsRoot string) (string, string, bool) {
	boundary := wsRoot
	if gitRoot, ok := workspace.FindGitRoot(cwd, wsRoot); ok {
		boundary = gitRoot
	} else if wsRoot == "" {
		boundary = cwd
	}
	for _, name := range []string{".node-version", ".nvmrc"} {
		path, ok := workspace.FindUpward(cwd, boundary, name)
		if !ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if v, ok := node.ParseVersionFile(string(data)); ok {
			return v, path, true
		}
	}
	return "", "", false
}

func runPlan(cmd *cobra.Command, plan *execPlan) {
	var c *exec.Cmd
	if plan.viaCmd {
		args := append([]string{"/c", plan.bin}, plan.passthrough...)
		c = exec.Command("cmd", args...)
	} else {
		c = exec.Command(plan.bin, plan.passthrough...)
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = childEnv(os.Environ(), plan.nodeHome, plan.pathEntry)
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
		Code:    output.CodeNodeExecFailed,
		Message: fmt.Sprintf("cannot start %s: %v", filepath.ToSlash(plan.bin), err),
	})
}

// childEnv returns the child process environment: unchanged when no
// registered installation was resolved, otherwise with NODE_HOME set and the
// installation bin dir prepended to PATH.
func childEnv(env []string, nodeHome, pathEntry string) []string {
	if nodeHome == "" {
		return env
	}
	out := make([]string, 0, len(env)+2)
	seenHome, seenPath := false, false
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:i]
		switch {
		case strings.EqualFold(k, "NODE_HOME"):
			if !seenHome {
				out = append(out, "NODE_HOME="+nodeHome)
				seenHome = true
			}
		case strings.EqualFold(k, "PATH"):
			if !seenPath {
				out = append(out, "PATH="+pathEntry+string(os.PathListSeparator)+kv[i+1:])
				seenPath = true
			}
		default:
			out = append(out, kv)
		}
	}
	if !seenHome {
		out = append(out, "NODE_HOME="+nodeHome)
	}
	if !seenPath {
		out = append(out, "PATH="+pathEntry)
	}
	return out
}

func printPlan(cmd *cobra.Command, plan *execPlan, p output.Palette) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		detail := map[string]any{
			"args":    plan.passthrough,
			"command": commandLine(plan),
		}
		if plan.nodeHome != "" {
			nodeDetail := map[string]any{
				"spec":    plan.nodeSpec,
				"name":    plan.nodeName,
				"version": plan.nodeVersion,
				"home":    filepath.ToSlash(plan.nodeHome),
				"bin":     filepath.ToSlash(plan.bin),
				"source":  plan.nodeSrc,
			}
			if plan.nodeFile != "" {
				nodeDetail["file"] = filepath.ToSlash(plan.nodeFile)
			}
			detail["node"] = nodeDetail
		} else {
			detail["node"] = map[string]any{"bin": filepath.ToSlash(plan.bin), "source": "ambient"}
		}
		if plan.wsRoot != "" {
			detail["workspace"] = filepath.ToSlash(plan.wsRoot)
		}
		if plan.repoName != "" {
			detail["repo"] = map[string]any{"name": plan.repoName, "path": filepath.ToSlash(plan.repoPath)}
		}
		name := plan.repoName
		if name == "" {
			name = plan.tool
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
	if plan.nodeHome != "" {
		src := plan.nodeSrc
		if plan.nodeFile != "" {
			src = fmt.Sprintf("%s, %s", src, filepath.ToSlash(plan.nodeFile))
		}
		_, _ = fmt.Fprintf(w, "node\t%s → %s %s [%s]\n", p.Cyan(plan.nodeSpec), plan.nodeName, plan.nodeVersion, src)
		_, _ = fmt.Fprintf(w, "NODE_HOME\t%s\n", filepath.ToSlash(plan.nodeHome))
	} else {
		_, _ = fmt.Fprintf(w, "node\t%s\n", p.Dim("(ambient, NODE_HOME/PATH left untouched)"))
	}
	_, _ = fmt.Fprintf(w, "bin\t%s\n", filepath.ToSlash(plan.bin))
	_, _ = fmt.Fprintf(w, "command\t%s\n", commandLine(plan))
	_ = w.Flush()
}

func commandLine(plan *execPlan) string {
	var parts []string
	if plan.viaCmd {
		parts = append(parts, "cmd", "/c")
	}
	parts = append(parts, plan.bin)
	for _, a := range plan.passthrough {
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
