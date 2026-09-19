package jdkcli

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/jdk"
	"barista/internal/output"
)

func majorArg(arg string) (int, error) {
	major, err := strconv.Atoi(arg)
	if err != nil || major < 1 {
		return 0, fmt.Errorf("invalid major version %q (want a positive integer)", arg)
	}
	return major, nil
}

func setDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use: "set-default <major> <name>",
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return comp.JdkMajors(), cobra.ShellCompDirectiveNoFileComp
			}
			if len(args) == 1 {
				return comp.JdkNames(), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Short: "Set the default JDK for a major version",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			if _, err := majorArg(args[0]); err != nil {
				return err
			}
			if !jdk.ValidName(args[1]) {
				return fmt.Errorf("invalid JDK name %q (want [a-z0-9][a-z0-9._-]*)", args[1])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			major, _ := majorArg(args[0])
			name := args[1]
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			entry, e := reg.SetDefault(major, name)
			if e != nil {
				failResult(cmd, p, output.Result{Name: name, Action: "set-default", Status: output.StatusFailed, Error: e})
				return nil
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res := output.Result{
				Name:   name,
				Path:   filepath.ToSlash(entry.Path),
				Status: output.StatusOK,
				Action: "set-default",
				Detail: map[string]any{"major": major},
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); !jsonOut {
				fmt.Printf("default for %d set to %s\n", major, p.Cyan(name))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
