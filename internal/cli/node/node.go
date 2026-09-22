package nodecli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/download"
	"barista/internal/node"
	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:                   "node [flags] -- <node args...>",
		Short:                 "Run Node.js with the workspace-aware environment, or manage Node.js installations",
		DisableFlagsInUseLine: true,
		Long: "Run node in the current directory. Arguments after \"--\" are passed through verbatim.\n" +
			"Resolution (high to low): --node > .node-version/.nvmrc file > repo properties[\"node\"] > workspace node > user node.json default > ambient PATH.\n" +
			"File detection can be disabled with the workspace property detect.files=false.\n" +
			"When a registered installation is resolved, node runs directly (no cmd wrapper on Windows) and the child process gets NODE_HOME plus the installation bin dir prepended to PATH.\n" +
			"Subcommand names win over passthrough arguments; the management subcommands are listed below.",
		Example: `  barista node -- --version
	  barista node --node 22 --dry-run -- server.js
	  barista node install 22.14.0`,
		Args: cobra.ArbitraryArgs,
		RunE: runExec,
	}
	cmd.Flags().String("node", "", "Node.js spec (registry name or version) used to run node; overrides every config level")
	_ = cmd.RegisterFlagCompletionFunc("node", comp.Fn(comp.NodeSpecs))
	cmd.Flags().Bool("dry-run", false, "print the resolved Node.js and full command line without executing")
	cmd.AddCommand(
		listCmd(),
		addCmd(),
		removeCmd(),
		setDefaultCmd(),
		setInstallDirCmd(),
		whichCmd(),
		pathCmd(),
		homeCmd(),
		installCmd(),
		availableCmd(),
		uninstallCmd(),
		useCmd(),
		envCmd(),
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

func loadRegistry(cmd *cobra.Command) (*node.Registry, string, bool) {
	p, err := node.RegistryPath()
	if err != nil {
		fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		return nil, "", false
	}
	reg, e := node.Load(p)
	if e != nil {
		fail(cmd, e)
		return nil, "", false
	}
	return reg, p, true
}

func userSettings(cmd *cobra.Command) (workspace.ConfigFile, output.Palette, bool) {
	cfg, err := workspace.LoadUserConfig()
	if err != nil {
		fail(cmd, configErrInfo(err))
		return workspace.ConfigFile{}, output.NewPalette(false), false
	}
	return cfg, output.NewPalette(output.ColorEnabled(cfg.Color), cfg.ColorProfile), true
}

func saveRegistry(cmd *cobra.Command, reg *node.Registry, path string) bool {
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
		fail(cmd, configErrInfo(err))
		return nil, false
	}
	return props, true
}

func findWorkspaceRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return workspace.FindWorkspaceRoot(cwd)
}

// resolveMirrorBase resolves the node download mirror: --mirror flag when
// explicitly set, otherwise the merged config; "" means the official source.
func resolveMirrorBase(cmd *cobra.Command) (base, raw string, ok bool) {
	mirrorRaw := ""
	if cmd.Flags().Changed("mirror") {
		mirrorRaw, _ = cmd.Flags().GetString("mirror")
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
			return "", "", false
		}
		cfg, err := workspace.LoadMergedConfig(cwd)
		if err != nil {
			fail(cmd, configErrInfo(err))
			return "", "", false
		}
		mirrorRaw = cfg.MirrorValueFor(download.DomainNode)
	}
	mirrorBase, err := download.MirrorBase(download.DomainNode, mirrorRaw)
	if err != nil {
		fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
		return "", "", false
	}
	if mirrorBase == "" {
		mirrorRaw = ""
	}
	return mirrorBase, mirrorRaw, true
}

type effectiveSettings struct {
	Default, DefaultSource string
}

func effective(reg *node.Registry, props workspace.Properties) effectiveSettings {
	var eff effectiveSettings
	if v, ok := props.String("node"); ok && v != "" {
		eff.Default, eff.DefaultSource = v, "workspace"
	} else if reg.Default != "" {
		eff.Default, eff.DefaultSource = reg.Default, "user"
	}
	return eff
}

func resolveEffectiveDefault(reg *node.Registry, eff effectiveSettings) (*node.Entry, string, *output.ErrInfo) {
	if eff.Default == "" {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeNodeNotFound,
			Message: "no default node configured",
			Hint:    "set one with: barista node set-default <name>",
		}
	}
	if eff.DefaultSource == "user" {
		if e := reg.Find(eff.Default); e != nil {
			return e, "default:user", nil
		}
		return nil, "", &output.ErrInfo{
			Code:    output.CodeNodeNotFound,
			Message: fmt.Sprintf("default node %q (from user config) is not registered", eff.Default),
			Hint:    "run: barista node list; fix with: barista node set-default <name>",
		}
	}
	e, _, re := reg.Resolve(eff.Default)
	if re != nil {
		return nil, "", &output.ErrInfo{
			Code:    output.CodeNodeNotFound,
			Message: fmt.Sprintf("node %q (from workspace config) is not registered", eff.Default),
			Hint:    "run: barista node list; fix the property or register the installation",
		}
	}
	return e, "default:workspace", nil
}

func autoName(reg *node.Registry, version string) string {
	return reg.AvailableName(node.NameFor(version))
}

func customNameError(name string) *output.ErrInfo {
	if !node.ValidName(name) {
		return &output.ErrInfo{
			Code:    output.CodeConfigError,
			Message: fmt.Sprintf("invalid node name %q (want [a-z0-9][a-z0-9._-]*)", name),
		}
	}
	if node.LooksLikeVersion(name) {
		return &output.ErrInfo{
			Code:    output.CodeConfigError,
			Message: fmt.Sprintf("node name %q must not look like a version (ambiguous in barista node which)", name),
		}
	}
	return nil
}
