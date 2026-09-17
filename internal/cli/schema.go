package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"barista/internal/workspace"
	"barista/schemas"
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
	for _, e := range schemas.List() {
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
			e, err := schemas.Get(args[0])
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
	Schema string          `json:"schema"`
	File   *string         `json:"file"`
	Valid  bool            `json:"valid"`
	Errors []schemas.Issue `json:"errors"`
}

type scopeResult struct {
	Scope  string          `json:"scope"`
	File   *string         `json:"file"`
	Valid  bool            `json:"valid"`
	Errors []schemas.Issue `json:"errors"`
}

type scopedValidateResult struct {
	Schema  string        `json:"schema"`
	Results []scopeResult `json:"results"`
}

func schemaValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <name> [file]",
		Short: "Validate a config file against an embedded schema",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := schemas.Get(name); err != nil {
				schemaUsageError(err)
				return nil
			}
			scope, _ := cmd.Flags().GetString("scope")
			if scope != "" && scope != "user" && scope != "workspace" {
				schemaEnvError(fmt.Sprintf("invalid --scope %q (want user|workspace)", scope))
				return nil
			}
			if scope != "" && name != "config" {
				schemaEnvError("--scope only applies to schema \"config\"")
				return nil
			}
			if len(args) == 2 {
				if scope != "" {
					schemaEnvError("--scope cannot be used with an explicit file")
					return nil
				}
				b, err := os.ReadFile(args[1])
				if err != nil {
					schemaEnvError(fmt.Sprintf("cannot read %s: %v", args[1], err))
					return nil
				}
				var filePath *string
				if abs, err := filepath.Abs(args[1]); err == nil {
					p := filepath.ToSlash(abs)
					filePath = &p
				}
				issues, envErr := validateData(name, b)
				if envErr != "" {
					schemaEnvError(envErr)
					return nil
				}
				writeValidateResult(validateResult{Schema: name, File: filePath, Valid: len(issues) == 0, Errors: issues})
				if len(issues) > 0 {
					ExitCode = 1
				}
				return nil
			}
			if name == "config" {
				validateConfigScopes(scope)
				return nil
			}
			cwd, _ := os.Getwd()
			root, err := workspace.FindRoot(cwd)
			if err != nil {
				schemaEnvError(err.Error())
				return nil
			}
			p := filepath.Join(root, ".barista", name+".json")
			b, err := os.ReadFile(p)
			if err != nil {
				schemaEnvError(fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(p), err))
				return nil
			}
			issues, envErr := validateData(name, b)
			if envErr != "" {
				schemaEnvError(envErr)
				return nil
			}
			sp := filepath.ToSlash(p)
			writeValidateResult(validateResult{Schema: name, File: &sp, Valid: len(issues) == 0, Errors: issues})
			if len(issues) > 0 {
				ExitCode = 1
			}
			return nil
		},
	}
	cmd.Flags().String("scope", "", "for config: validate only this level (user|workspace)")
	return cmd
}

func validateData(name string, data []byte) (issues []schemas.Issue, envErr string) {
	issues, err := schemas.Validate(name, data)
	if err != nil {
		return nil, err.Error()
	}
	if issues == nil {
		issues = []schemas.Issue{}
	}
	return issues, ""
}

func validateConfigScopes(scope string) {
	var results []scopeResult
	fail := false
	if scope == "" || scope == "user" {
		up, err := workspace.UserConfigPath()
		if err != nil {
			schemaEnvError(err.Error())
			return
		}
		r, envErr := validateScopeFile("user", up)
		if envErr != "" {
			schemaEnvError(envErr)
			return
		}
		results = append(results, r)
	}
	if scope == "" || scope == "workspace" {
		cwd, _ := os.Getwd()
		root, err := workspace.FindRoot(cwd)
		if err != nil {
			schemaEnvError(err.Error())
			return
		}
		r, envErr := validateScopeFile("workspace", filepath.Join(root, ".barista", "config.json"))
		if envErr != "" {
			schemaEnvError(envErr)
			return
		}
		results = append(results, r)
	}
	for _, r := range results {
		if !r.Valid {
			fail = true
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(scopedValidateResult{Schema: "config", Results: results})
	if fail {
		ExitCode = 1
	}
}

func validateScopeFile(scope, path string) (scopeResult, string) {
	r := scopeResult{Scope: scope, Errors: []schemas.Issue{}, Valid: true}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, ""
		}
		return r, fmt.Sprintf("cannot read %s: %v", filepath.ToSlash(path), err)
	}
	sp := filepath.ToSlash(path)
	r.File = &sp
	issues, envErr := validateData("config", data)
	if envErr != "" {
		return r, envErr
	}
	r.Errors = issues
	r.Valid = len(issues) == 0
	return r, ""
}

func writeValidateResult(r validateResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(r)
}

func schemaUsageError(err error) {
	fmt.Fprintf(os.Stderr, "barista: %v\n", err)
	fmt.Fprintln(os.Stderr, "available schemas:")
	for _, e := range schemas.List() {
		fmt.Fprintf(os.Stderr, "  %s\n", e.Name)
	}
	ExitCode = 2
}

func schemaEnvError(msg string) {
	fmt.Fprintf(os.Stderr, "barista: %s\n", msg)
	ExitCode = 2
}
