package repocli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"barista/internal/gitrun"
	"barista/internal/output"
	"barista/internal/workspace"
)

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <url>",
		Short: "Register a repository in repos.json and clone it",
		Long: "Register a repository in .barista/repos.json and clone it into the workspace.\n" +
			"The name is derived from the URL basename unless --name is given; the checkout path\n" +
			"defaults to repos/<name>. URLs prefixed by the manifest's baseUrl are stored relative.\n" +
			"Idempotent: re-adding the same repo skips registration and only retries the clone.",
		Example: `  barista repo add git@github.com:org/order-service.git
  barista repo add order-service --label java --no-clone`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			ws, ok := loadWorkspace(cmd)
			if !ok {
				return nil
			}
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				name = nameFromURL(args[0])
			}
			if name == "" {
				fail(cmd, &output.ErrInfo{
					Code:    output.CodeUsageError,
					Message: fmt.Sprintf("cannot derive a repo name from %q", args[0]),
					Hint:    "pass one explicitly with --name",
				})
				return nil
			}
			relPath, _ := cmd.Flags().GetString("path")
			labels, _ := cmd.Flags().GetStringSlice("label")
			entry, appended, err := workspace.AddRepo(ws.Root, name, args[0], relPath, labels)
			if err != nil {
				e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
				var le *workspace.LoadError
				if errors.As(err, &le) {
					e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
				}
				fail(cmd, e)
				return nil
			}
			entry.ResolvedURL = workspace.ResolveURL(ws.Repos.BaseURL, entry.URL)
			res := output.Result{
				Name:   entry.Name,
				Path:   entry.Path,
				Status: output.StatusOK,
				Action: "add",
				Detail: map[string]any{
					"registered":  appended,
					"url":         entry.URL,
					"resolvedUrl": entry.ResolvedURL,
				},
			}
			if len(entry.Labels) > 0 {
				res.Detail["labels"] = entry.Labels
			}
			if noClone, _ := cmd.Flags().GetBool("no-clone"); noClone {
				res.Detail["cloned"] = "deferred"
				emit(cmd, ws, res)
				return nil
			}
			if e := verifyOrigin(cmd, ws, entry); e != nil {
				res.Status = output.StatusFailed
				res.Error = e
				emit(cmd, ws, res)
				*exitCode = 1
				return nil
			}
			clone := gitrun.Clone(cmd.Context(), ws, entry)
			res.Branch = clone.Branch
			res.DurationMs = clone.DurationMs
			switch clone.Status {
			case output.StatusFailed:
				res.Status = output.StatusFailed
				res.Error = clone.Error
				res.Detail["cloned"] = "failed"
			case output.StatusSkipped:
				res.Detail["cloned"] = "exists"
			default:
				res.Detail["cloned"] = "cloned"
			}
			emit(cmd, ws, res)
			*exitCode = output.ExitCode([]output.Result{res})
			return nil
		},
	}
	cmd.Flags().String("name", "", "repo name in the manifest (default: derived from the URL basename)")
	cmd.Flags().String("path", "", "checkout path relative to the workspace root (default: repos/<name>)")
	cmd.Flags().StringSlice("label", nil, "labels for --label filtering (repeatable)")
	cmd.Flags().Bool("no-clone", false, "register only; clone later with: barista git clone")
	return cmd
}

func nameFromURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	return u
}

func verifyOrigin(cmd *cobra.Command, ws *workspace.Workspace, entry workspace.Repo) *output.ErrInfo {
	dir := ws.AbsPath(entry)
	if !gitrun.IsGitRepo(cmd.Context(), dir) {
		return nil
	}
	origin, err := gitrun.OriginURL(cmd.Context(), dir)
	if err != nil {
		return &output.ErrInfo{Code: output.CodeGitError, Message: fmt.Sprintf("cannot read origin of %s: %v", filepath.ToSlash(dir), err)}
	}
	if workspace.NormalizeURL(origin) != workspace.NormalizeURL(entry.ResolvedURL) {
		return &output.ErrInfo{
			Code:    output.CodeRepoRemoteMismatch,
			Message: fmt.Sprintf("%s is a git repo with origin %q, but the manifest resolves to %q", filepath.ToSlash(dir), origin, entry.ResolvedURL),
			Hint:    "fix the url in .barista/repos.json or remove the checkout",
		}
	}
	return nil
}

func emit(cmd *cobra.Command, ws *workspace.Workspace, res output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), filepath.ToSlash(ws.Root), []output.Result{res}))
		return
	}
	if res.Status == output.StatusFailed {
		fmt.Fprintf(os.Stderr, "barista: %s: %s\n", res.Error.Code, res.Error.Message)
		if res.Error.Hint != "" {
			fmt.Fprintf(os.Stderr, "hint: %s\n", res.Error.Hint)
		}
		return
	}
	registered := "registered"
	if r, _ := res.Detail["registered"].(bool); !r {
		registered = "already registered"
	}
	switch res.Detail["cloned"] {
	case "deferred":
		fmt.Printf("%s %s → %s (clone deferred)\n", registered, res.Name, filepath.ToSlash(res.Path))
	case "exists":
		fmt.Printf("%s %s → %s (already cloned)\n", registered, res.Name, filepath.ToSlash(res.Path))
	default:
		fmt.Printf("%s %s → %s, cloned from %s (%s)\n", registered, res.Name, filepath.ToSlash(res.Path), res.Detail["resolvedUrl"], res.Branch)
	}
}
