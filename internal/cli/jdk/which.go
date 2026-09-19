package jdkcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/output"
)

func whichCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "which <major|name>",
		Short: "Resolve a JDK by major version (default first, else latest) or by exact name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathOnly, _ := cmd.Flags().GetBool("pathonly")
			runResolve(cmd, args[0], pathOnly)
			return nil
		},
	}
	cmd.Flags().Bool("pathonly", false, "print only the JDK path, without a trailing newline")
	return cmd
}

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "path <major|name>",
		ValidArgsFunction: comp.Fn(comp.JdkSpecs),
		Short:             "Print the resolved JDK path (shortcut for: which --pathonly)",
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runResolve(cmd, args[0], true)
			return nil
		},
	}
}

func homeCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "home <major|name>",
		ValidArgsFunction: comp.Fn(comp.JdkSpecs),
		Short:             "Print the resolved JDK path (same as: path)",
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runResolve(cmd, args[0], true)
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
	entry, source, e := reg.Resolve(arg)
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
			"major":   entry.Major,
			"version": entry.Version,
			"managed": entry.Managed,
			"source":  source,
		},
	}
	_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), "", []output.Result{res}))
}
