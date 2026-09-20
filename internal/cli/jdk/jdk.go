package jdkcli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "jdk",
		Short: "Manage registered JDK installations",
	}
	cmd.AddCommand(
		listCmd(),
		addCmd(),
		removeCmd(),
		setDefaultCmd(),
		setInstallDirCmd(),
		useCmd(),
		whichCmd(),
		pathCmd(),
		homeCmd(),
		envCmd(),
		discoverCmd(),
		installCmd(),
		downloadCmd(),
		availableCmd(),
		uninstallCmd(),
	)
	return cmd
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

func loadRegistry(cmd *cobra.Command) (*jdk.Registry, string, bool) {
	p, err := jdk.RegistryPath()
	if err != nil {
		fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		return nil, "", false
	}
	reg, e := jdk.Load(p)
	if e != nil {
		fail(cmd, e)
		return nil, "", false
	}
	return reg, p, true
}

func userSettings(cmd *cobra.Command) (workspace.ConfigFile, output.Palette, bool) {
	cfg, err := workspace.LoadUserConfig()
	if err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		fail(cmd, e)
		return workspace.ConfigFile{}, output.NewPalette(false), false
	}
	return cfg, output.NewPalette(output.ColorEnabled(cfg.Color)), true
}

func saveRegistry(cmd *cobra.Command, reg *jdk.Registry, path string) bool {
	if e := reg.Save(path); e != nil {
		fail(cmd, e)
		return false
	}
	return true
}

func finish(cmd *cobra.Command, results []output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), "", results))
	}
	*exitCode = output.ExitCode(results)
}

func failResult(cmd *cobra.Command, p output.Palette, res output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
		_, _ = fmt.Fprintln(os.Stderr, p.Red(fmt.Sprintf("failed %s: %s: %s", res.Name, res.Error.Code, res.Error.Message)))
		if res.Error.Hint != "" {
			_, _ = fmt.Fprintln(os.Stderr, p.Dim("hint: "+res.Error.Hint))
		}
	}
	finish(cmd, []output.Result{res})
}
