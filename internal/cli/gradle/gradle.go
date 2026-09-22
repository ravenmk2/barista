package gradlecli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/gradle"
	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:                   "gradle [flags] -- <gradle args...>",
		Short:                 "Run Gradle with the workspace-aware environment, or manage Gradle installations",
		DisableFlagsInUseLine: true,
		Long: "Run Gradle in the current directory. Arguments after \"--\" are passed through verbatim.\n" +
			"Resolution (high to low): JDK: --jdk > .java-version file > repo properties[\"jdk\"] > workspace jdk > user gradle.json jdk > ambient.\n" +
			"Gradle installation: gradle/wrapper/gradle-wrapper.properties version > workspace gradle.default > user gradle.json default.\n" +
			"File detection can be disabled with the workspace property detect.files=false.\n" +
			"init scripts: .barista/gradle/init.gradle and .barista/gradle/init.gradle.kts are injected as -I when present (skipped when you pass -I yourself).\n" +
			"gradle.user.home: workspace property injected as --gradle-user-home (skipped when you pass it yourself).\n" +
			"Gradle is always started via its launcher script (bin/gradle, or cmd /c bin/gradle.bat on Windows).\n" +
			"Subcommand names win over passthrough arguments; the management subcommands are listed below.",
		Example: `  barista gradle -- build test
	  barista gradle --jdk 17 --dry-run -- -q assemble
	  barista gradle install 8.10.2`,
		Args: cobra.ArbitraryArgs,
		RunE: runExec,
	}
	cmd.Flags().String("jdk", "", "JDK spec (registry name or major version) used to run Gradle; overrides every config level")
	_ = cmd.RegisterFlagCompletionFunc("jdk", comp.Fn(comp.JdkSpecs))
	cmd.Flags().Bool("dry-run", false, "print the resolved environment and full command line without executing")
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
		availableCmd(),
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

func configErrInfo(err error) *output.ErrInfo {
	e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	var le *workspace.LoadError
	if errors.As(err, &le) {
		e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
	}
	return e
}

func loadRegistry(cmd *cobra.Command) (*gradle.Registry, string, bool) {
	p, err := gradle.RegistryPath()
	if err != nil {
		fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		return nil, "", false
	}
	reg, e := gradle.Load(p)
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
	return cfg, output.NewPalette(output.ColorEnabled(cfg.Color), cfg.ColorProfile), true
}

func saveRegistry(cmd *cobra.Command, reg *gradle.Registry, path string) bool {
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

func workspaceProperties(cmd *cobra.Command) (workspace.Properties, bool) {
	root := findWorkspaceRoot()
	if root == "" {
		return nil, true
	}
	props, err := workspace.LoadProperties(filepath.Join(root, ".barista", "properties.json"))
	if err != nil {
		e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		var le *workspace.LoadError
		if errors.As(err, &le) {
			e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
		}
		fail(cmd, e)
		return nil, false
	}
	return props, true
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
	cmd.Flags().String("scope", "user", "where to write: user (~/.barista/gradle.json) or workspace (<workspace>/.barista/properties.json)")
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
	p := filepath.Join(root, ".barista", "properties.json")
	if err := workspace.SetProperty(p, key, value); err != nil {
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

func effective(reg *gradle.Registry, props workspace.Properties) effectiveSettings {
	var eff effectiveSettings
	if v, ok := props.String("gradle.default"); ok && v != "" {
		eff.Default, eff.DefaultSource = v, "workspace"
	} else if reg.Default != "" {
		eff.Default, eff.DefaultSource = reg.Default, "user"
	}
	if v, ok := props.String("jdk"); ok && v != "" {
		eff.Jdk, eff.JdkSource = v, "workspace"
	} else if reg.Jdk != "" {
		eff.Jdk, eff.JdkSource = reg.Jdk, "user"
	}
	return eff
}

func resolveEffectiveDefault(reg *gradle.Registry, eff effectiveSettings) (*gradle.Entry, string, *output.ErrInfo) {
	if eff.Default == "" {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeGradleNotFound,
			Message: "no default gradle configured",
			Hint:    "set one with: barista gradle set-default <name>",
		}
	}
	if e := reg.Find(eff.Default); e != nil {
		return e, "default:" + eff.DefaultSource, nil
	}
	return nil, "", &output.ErrInfo{
		Code:    output.CodeGradleNotFound,
		Message: fmt.Sprintf("default gradle %q (from %s config) is not registered", eff.Default, eff.DefaultSource),
		Hint:    "run: barista gradle list; fix with: barista gradle set-default <name> --scope " + eff.DefaultSource,
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

func autoName(reg *gradle.Registry, version string) string {
	return reg.AvailableName(gradle.NameFor(version))
}

func customNameError(name string) *output.ErrInfo {
	if !gradle.ValidName(name) {
		return &output.ErrInfo{
			Code:    output.CodeConfigError,
			Message: fmt.Sprintf("invalid gradle name %q (want [a-z0-9][a-z0-9._-]*)", name),
		}
	}
	if gradle.LooksLikeVersion(name) {
		return &output.ErrInfo{
			Code:    output.CodeConfigError,
			Message: fmt.Sprintf("gradle name %q must not look like a version (ambiguous in barista gradle which)", name),
		}
	}
	return nil
}
