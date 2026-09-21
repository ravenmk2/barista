package initcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:               "init [path]",
		ValidArgsFunction: comp.Dirs,
		Short:             "Bootstrap a barista workspace",
		Long: "Create .barista/repos.json in the target directory (default: current directory).\n" +
			"An existing repos.json is never overwritten. With --scan, git checkouts up to two levels\n" +
			"below the target (e.g. repos/<name>) are imported with their origin URLs; entries whose\n" +
			"name is already registered are skipped.",
		Example: `  barista init --repo-base-url git@github.com:org/
  barista init D:\work\ws --scan`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			target := ""
			if len(args) == 1 {
				target = args[0]
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
					return nil
				}
				target = cwd
			}
			abs, err := filepath.Abs(target)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			baseURL, _ := cmd.Flags().GetString("repo-base-url")
			branch, _ := cmd.Flags().GetString("default-branch")
			created, err := workspace.Init(abs, baseURL, branch)
			if err != nil {
				fail(cmd, loadErr(err))
				return nil
			}
			res := output.Result{
				Name:   filepath.Base(abs),
				Path:   filepath.ToSlash(abs),
				Status: output.StatusOK,
				Action: "init",
				Detail: map[string]any{
					"created":   created,
					"reposJson": filepath.ToSlash(filepath.Join(abs, ".barista", "repos.json")),
				},
			}
			if !created {
				res.Detail["reason"] = "already initialized"
			}
			if scan, _ := cmd.Flags().GetBool("scan"); scan {
				imported, skipped := scanImport(cmd, abs)
				res.Detail["imported"] = imported
				res.Detail["skipped"] = skipped
			}
			emit(cmd, res)
			return nil
		},
	}
	cmd.Flags().String("repo-base-url", "", "prefix for relative repo URLs in the manifest")
	cmd.Flags().String("default-branch", "", "fallback default branch for all repos")
	cmd.Flags().Bool("scan", false, "import existing git checkouts (up to two levels deep) by their origin URLs")
	return cmd
}

type importedRepo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Path string `json:"path"`
}

type skippedRepo struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func scanImport(cmd *cobra.Command, root string) ([]importedRepo, []skippedRepo) {
	var imported []importedRepo
	var skipped []skippedRepo
	rels, err := workspace.ScanCheckouts(root)
	if err != nil {
		fail(cmd, loadErr(err))
		return nil, nil
	}
	for _, rel := range rels {
		origin, err := gitrun.OriginURL(cmd.Context(), filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || origin == "" {
			skipped = append(skipped, skippedRepo{Path: rel, Reason: "no origin remote"})
			continue
		}
		name := filepath.Base(rel)
		entry, appended, err := workspace.AddRepo(root, name, origin, rel, nil)
		if err != nil {
			skipped = append(skipped, skippedRepo{Path: rel, Reason: err.Error()})
			continue
		}
		if !appended {
			skipped = append(skipped, skippedRepo{Path: rel, Reason: "already registered"})
			continue
		}
		imported = append(imported, importedRepo{Name: entry.Name, URL: entry.URL, Path: entry.Path})
	}
	return imported, skipped
}

func emit(cmd *cobra.Command, res output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), res.Path, []output.Result{res}))
		return
	}
	cfg, err := workspace.LoadUserConfig()
	p := output.NewPalette(err == nil && output.ColorEnabled(cfg.Color), cfg.ColorProfile)
	if res.Detail["created"] == true {
		fmt.Printf("%s %s\n", p.Green("initialized"), res.Detail["reposJson"])
	} else {
		fmt.Printf("%s %s\n", p.Dim("already initialized"), res.Detail["reposJson"])
	}
	if imported, ok := res.Detail["imported"].([]importedRepo); ok {
		for _, r := range imported {
			fmt.Printf("  %s %s ← %s (%s)\n", p.Green("imported"), p.Cyan(r.Name), r.URL, r.Path)
		}
	}
	if skipped, ok := res.Detail["skipped"].([]skippedRepo); ok {
		for _, s := range skipped {
			fmt.Printf("  %s %s (%s)\n", p.Dim("skipped"), s.Path, p.Dim(s.Reason))
		}
	}
}

func loadErr(err error) *output.ErrInfo {
	e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	var le *workspace.LoadError
	if errors.As(err, &le) {
		e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
	}
	return e
}

func fail(cmd *cobra.Command, e *output.ErrInfo) {
	p := output.NewPalette(false)
	cfg, err := workspace.LoadUserConfig()
	if err == nil {
		p = output.NewPalette(output.ColorEnabled(cfg.Color), cfg.ColorProfile)
	}
	fmt.Fprintf(os.Stderr, "%s %s\n", p.Red("barista: "+e.Code+":"), e.Message)
	if e.Hint != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", p.Yellow("hint:"), e.Hint)
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
	}
	*exitCode = 2
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
}
