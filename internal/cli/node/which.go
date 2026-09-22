package nodecli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/node"
	"barista/internal/output"
)

func whichCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "which [name|version]",
		ValidArgsFunction: comp.Fn(comp.NodeSpecs),
		Short:             "Resolve a Node.js installation by name, version (progressively widened), or the effective default",
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathOnly, _ := cmd.Flags().GetBool("pathonly")
			runResolve(cmd, firstArg(args), pathOnly)
			return nil
		},
	}
	cmd.Flags().Bool("pathonly", false, "print only the node home path, without a trailing newline")
	return cmd
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "path [name|version]",
		ValidArgsFunction: comp.Fn(comp.NodeSpecs),
		Short:             "Print the resolved node home path (shortcut for: which --pathonly)",
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runResolve(cmd, firstArg(args), true)
			return nil
		},
	}
}

func homeCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "home [name|version]",
		ValidArgsFunction: comp.Fn(comp.NodeSpecs),
		Short:             "Print the resolved node home path (same as: path)",
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runResolve(cmd, firstArg(args), true)
			return nil
		},
	}
}

func runResolve(cmd *cobra.Command, arg string, pathOnly bool) {
	*exitCode = 0
	reg, _, ok := loadRegistry(cmd)
	if !ok {
		return
	}
	var entry *node.Entry
	var source string
	var e *output.ErrInfo
	if arg != "" {
		entry, source, e = reg.Resolve(arg)
	} else {
		props, ok := workspaceProperties(cmd)
		if !ok {
			return
		}
		entry, source, e = resolveEffectiveDefault(reg, effective(reg, props))
	}
	if e != nil {
		fmt.Fprintf(os.Stderr, "barista: %s: %s\n", e.Code, e.Message)
		if e.Hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
		}
		if !pathOnly {
			_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
		}
		*exitCode = 1
		return
	}
	if pathOnly {
		fmt.Print(entry.Path)
		return
	}
	res := output.Result{
		Name:   entry.Name,
		Path:   filepath.ToSlash(entry.Path),
		Status: output.StatusOK,
		Action: "which",
		Detail: map[string]any{
			"version": entry.Version,
			"managed": entry.Managed,
			"source":  source,
		},
	}
	_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), "", []output.Result{res}))
}
