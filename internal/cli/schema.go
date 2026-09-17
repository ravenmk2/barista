package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista"
	"barista/internal/workspace"
)

func schemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Inspect embedded JSON Schemas for barista config files",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			printSchemaList(os.Stdout)
			return nil
		},
	}
	cmd.AddCommand(schemaListCmd(), schemaShowCmd(), schemaValidateCmd())
	return cmd
}

func printSchemaList(w io.Writer) {
	fmt.Fprintln(w, "Available schemas:")
	for _, e := range barista.List() {
		fmt.Fprintf(w, "  %-10s %s\n", e.Name, e.Description)
	}
}

func schemaListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available schemas",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			printSchemaList(os.Stdout)
			return nil
		},
	}
}

func schemaShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Print a schema as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e, err := barista.Get(args[0])
			if err != nil {
				schemaUsageError(err)
				return nil
			}
			_, err = os.Stdout.Write(e.Raw)
			return err
		},
	}
}

type validateResult struct {
	Schema string        `json:"schema"`
	File   *string       `json:"file"`
	Valid  bool          `json:"valid"`
	Errors []barista.Issue `json:"errors"`
}

func schemaValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <name> [file]",
		Short: "Validate a config file against an embedded schema",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if _, err := barista.Get(name); err != nil {
				schemaUsageError(err)
				return nil
			}
			var data []byte
			var filePath *string
			if len(args) == 2 {
				b, err := os.ReadFile(args[1])
				if err != nil {
					schemaEnvError(fmt.Sprintf("cannot read %s: %v", args[1], err))
					return nil
				}
				data = b
				abs, err := filepath.Abs(args[1])
				if err == nil {
					p := filepath.ToSlash(abs)
					filePath = &p
				}
			} else {
				cwd, _ := os.Getwd()
				root, err := workspace.FindRoot(cwd)
				if err != nil {
					schemaEnvError(err.Error())
					return nil
				}
				p := filepath.Join(root, ".barista", name+".json")
				b, err := os.ReadFile(p)
				if err != nil {
					if name == "config" && os.IsNotExist(err) {
						writeValidateResult(validateResult{Schema: name, File: nil, Valid: true, Errors: []barista.Issue{}})
						return nil
					}
					schemaEnvError(fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(p), err))
					return nil
				}
				data = b
				sp := filepath.ToSlash(p)
				filePath = &sp
			}
			issues, err := barista.Validate(name, data)
			if err != nil {
				schemaEnvError(err.Error())
				return nil
			}
			if issues == nil {
				issues = []barista.Issue{}
			}
			writeValidateResult(validateResult{Schema: name, File: filePath, Valid: len(issues) == 0, Errors: issues})
			if len(issues) > 0 {
				ExitCode = 1
			}
			return nil
		},
	}
}

func writeValidateResult(r validateResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(r)
}

func schemaUsageError(err error) {
	fmt.Fprintf(os.Stderr, "barista: %v\n", err)
	fmt.Fprintln(os.Stderr, "available schemas:")
	for _, e := range barista.List() {
		fmt.Fprintf(os.Stderr, "  %s\n", e.Name)
	}
	ExitCode = 2
}

func schemaEnvError(msg string) {
	fmt.Fprintf(os.Stderr, "barista: %s\n", msg)
	ExitCode = 2
}
