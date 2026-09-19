package mavencli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		Use:   "maven",
		Short: "Manage registered Maven installations",
	}
	cmd.AddCommand(
		listCmd(),
		addCmd(),
		removeCmd(),
		setDefaultCmd(),
		setJdkCmd(),
		setInstallDirCmd(),
		whichCmd(),
		pathCmd(),
		homeCmd(),
		discoverCmd(),
		installCmd(),
		uninstallCmd(),
		configCmd(),
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

func loadRegistry(cmd *cobra.Command) (*maven.Registry, string, bool) {
	p, err := maven.RegistryPath()
	if err != nil {
		fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		return nil, "", false
	}
	reg, e := maven.Load(p)
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

func saveRegistry(cmd *cobra.Command, reg *maven.Registry, path string) bool {
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
		_, _ = fmt.Fprintln(os.Stdout, p.Red(fmt.Sprintf("failed %s: %s: %s", res.Name, res.Error.Code, res.Error.Message)))
		if res.Error.Hint != "" {
			_, _ = fmt.Fprintln(os.Stdout, p.Dim("hint: "+res.Error.Hint))
		}
	}
	finish(cmd, []output.Result{res})
}

func workspaceConfig(cmd *cobra.Command) (workspace.ConfigFile, bool) {
	root := findWorkspaceRoot()
	if root == "" {
		return workspace.ConfigFile{}, true
	}
	cf, err := workspace.LoadConfigFile(filepath.Join(root, ".barista", "config.json"))
	if err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		fail(cmd, e)
		return workspace.ConfigFile{}, false
	}
	return cf, true
}

func requireWorkspaceRoot(cmd *cobra.Command) (string, bool) {
	root := findWorkspaceRoot()
	if root == "" {
		fail(cmd, &output.ErrInfo{
			Code:    output.CodeWorkspaceNotFound,
			Message: "no .barista workspace found in current directory or any parent",
			Hint:    "run inside a barista workspace, or use --scope user",
		})
		return "", false
	}
	return root, true
}

func findWorkspaceRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return workspace.FindWorkspaceRoot(cwd)
}

func addScopeFlag(cmd *cobra.Command) {
	cmd.Flags().String("scope", "user", "where to write: user (~/.barista/maven.json) or workspace (<workspace>/.barista/config.json properties)")
}

func scopeOf(cmd *cobra.Command) (string, bool) {
	s, _ := cmd.Flags().GetString("scope")
	if s != "user" && s != "workspace" {
		fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: fmt.Sprintf("invalid --scope %q (want user|workspace)", s)})
		return "", false
	}
	return s, true
}

func writeWorkspaceProperty(cmd *cobra.Command, key, value string) bool {
	root, ok := requireWorkspaceRoot(cmd)
	if !ok {
		return false
	}
	p := filepath.Join(root, ".barista", "config.json")
	if err := workspace.SetConfigProperty(p, key, value); err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		fail(cmd, e)
		return false
	}
	return true
}

type effectiveSettings struct {
	Default, DefaultSource string
	Jdk, JdkSource         string
}

func effective(reg *maven.Registry, wsCfg workspace.ConfigFile) effectiveSettings {
	var eff effectiveSettings
	if v, ok := wsCfg.Property("maven.default"); ok && v != "" {
		eff.Default, eff.DefaultSource = v, "workspace"
	} else if reg.Default != "" {
		eff.Default, eff.DefaultSource = reg.Default, "user"
	}
	if v, ok := wsCfg.Property("maven.jdk"); ok && v != "" {
		eff.Jdk, eff.JdkSource = v, "workspace"
	} else if reg.Jdk != "" {
		eff.Jdk, eff.JdkSource = reg.Jdk, "user"
	}
	return eff
}

func resolveEffectiveDefault(reg *maven.Registry, eff effectiveSettings) (*maven.Entry, string, *output.ErrInfo) {
	if eff.Default == "" {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeMavenNotFound,
			Message: "no default maven configured",
			Hint:    "set one with: barista maven set-default <name>",
		}
	}
	if e := reg.Find(eff.Default); e != nil {
		return e, "default:" + eff.DefaultSource, nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeMavenNotFound,
		Message: fmt.Sprintf("default maven %q (from %s config) is not registered", eff.Default, eff.DefaultSource),
		Hint:    "run: barista maven list; fix with: barista maven set-default <name> --scope " + eff.DefaultSource,
	}
}

func resolveJdkSpec(cmd *cobra.Command, spec string) (*jdk.Entry, *output.ErrInfo) {
	p, err := jdk.RegistryPath()
	if err != nil {
		return nil, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	reg, e := jdk.Load(p)
	if e != nil {
		return nil, e
	}
	entry, _, re := reg.Resolve(spec)
	if re != nil {
		return nil, re
	}
	return entry, nil
}

func autoName(reg *maven.Registry, version string) string {
	return reg.AvailableName("maven-" + maven.TwoSegment(version))
}
