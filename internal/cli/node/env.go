package nodecli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/node"
	"barista/internal/output"
)

func envCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "env <name|version>",
		ValidArgsFunction: comp.Fn(comp.NodeSpecs),
		Short:             "Print NODE_HOME/PATH export statements for shell eval or CI",
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			shell := resolveShell(cmd)
			reg, _, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			entry, source, e := reg.Resolve(args[0])
			if e != nil {
				fmt.Fprintf(os.Stderr, "barista: %s: %s\n", e.Code, e.Message)
				if e.Hint != "" {
					fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
				}
				if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
					_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
				}
				*exitCode = 1
				return nil
			}
			text, e := renderEnv(shell, entry.Path)
			if e != nil {
				fail(cmd, e)
				return nil
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				res := output.Result{
					Name:   entry.Name,
					Path:   filepath.ToSlash(entry.Path),
					Status: output.StatusOK,
					Action: "env",
					Detail: map[string]any{
						"version":  entry.Version,
						"managed":  entry.Managed,
						"source":   source,
						"nodeHome": filepath.ToSlash(entry.Path),
						"bin":      filepath.ToSlash(node.BinDir(entry.Path)),
						"shell":    shell,
					},
				}
				_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), "", []output.Result{res}))
				return nil
			}
			fmt.Print(text)
			return nil
		},
	}
	cmd.Flags().String("shell", "", "output syntax: sh, cmd or powershell (aliases: bash/zsh, pwsh/ps); default: auto-detect, else platform default")
	_ = cmd.RegisterFlagCompletionFunc("shell", comp.Fn(comp.Shells))
	return cmd
}

func resolveShell(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("shell")
	if cmd.Flags().Changed("shell") {
		if s := normalizeShell(v); s != "" {
			return s
		}
		return v
	}
	if s := detectShell(runtime.GOOS, os.Getenv, parentExeName); s != "" {
		return s
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
}

func detectShell(goos string, getenv func(string) string, parentName func() string) string {
	if goos == "windows" {
		if s := normalizeShell(parentName()); s != "" {
			return s
		}
		if getenv("MSYSTEM") != "" {
			return "sh"
		}
		if s := normalizeShell(getenv("SHELL")); s == "sh" {
			return s
		}
		return ""
	}
	return normalizeShell(getenv("SHELL"))
}

func normalizeShell(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, ".exe")
	switch s {
	case "sh", "bash", "zsh":
		return "sh"
	case "cmd":
		return "cmd"
	case "powershell", "pwsh", "ps":
		return "powershell"
	}
	return ""
}

func renderEnv(shell, nodeHome string) (string, *output.ErrInfo) {
	binSh, binNative := "/bin", `\bin`
	if runtime.GOOS == "windows" {
		binSh, binNative = "", ""
	}
	switch shell {
	case "sh":
		return "export NODE_HOME='" + strings.ReplaceAll(nodeHome, `'`, `'\''`) + "'\n" +
			`export PATH="$NODE_HOME` + binSh + `:$PATH"` + "\n", nil
	case "cmd":
		return `set "NODE_HOME=` + nodeHome + `"` + "\r\n" +
			`set "PATH=%NODE_HOME%` + binNative + `;%PATH%"` + "\r\n", nil
	case "powershell":
		return "$env:NODE_HOME = '" + strings.ReplaceAll(nodeHome, `'`, `''`) + "'\n" +
			`$env:PATH = "$env:NODE_HOME` + binNative + `;$env:PATH"` + "\n", nil
	}
	return "", &output.ErrInfo{
		Code:    output.CodeUsageError,
		Message: fmt.Sprintf("unsupported shell %q", shell),
		Hint:    "valid values: sh, cmd, powershell (aliases: bash/zsh, pwsh/ps)",
	}
}
