package jdkcli

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

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
		Use:   "set-default <major> <name>",
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
			major, _ := majorArg(args[0])
			name := args[1]
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			entry, e := reg.SetDefault(major, name)
			if e != nil {
				failResult(cmd, output.Result{Name: name, Action: "set-default", Status: output.StatusFailed, Error: e})
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
				fmt.Printf("default for %d set to %s\n", major, name)
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
}
