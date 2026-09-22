package nodecli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/download"
	"barista/internal/node"
	"barista/internal/output"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <version>",
		Short: "Download Node.js (sha256-verified) from nodejs.org into the managed install dir",
		Example: `  barista node install 22.14.0
	  barista node install 22
	  barista node install v24`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if _, err := node.NormalizeVersion(args[0]); err != nil {
				return fmt.Errorf("invalid version %q (want e.g. 22.14.0)", args[0])
			}
			if n, _ := cmd.Flags().GetInt("attempts"); n < 1 {
				return fmt.Errorf("invalid --attempts %d (want >= 1)", n)
			}
			if cmd.Flags().Changed("mirror") {
				v, _ := cmd.Flags().GetString("mirror")
				if err := download.ValidateMirror(download.DomainNode, v); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			_, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			version := args[0]
			name, _ := cmd.Flags().GetString("name")
			res := output.Result{Action: "install"}
			failRes := func(e *output.ErrInfo) {
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, p, res)
			}
			if name != "" {
				res.Name = name
				if e := customNameError(name); e != nil {
					failRes(e)
					return nil
				}
				if reg.Find(name) != nil {
					failRes(&output.ErrInfo{
						Code:    output.CodeNodeExists,
						Message: fmt.Sprintf("name %q is already registered", name),
					})
					return nil
				}
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			versions, e := node.Available(cmd.Context())
			if e != nil {
				failRes(e)
				return nil
			}
			match, found := node.MatchAvailable(versions, version)
			if !found {
				failRes(&output.ErrInfo{
					Code:    output.CodeNodeNotFound,
					Message: fmt.Sprintf("no node version matching %q on nodejs.org", version),
					Hint:    "list versions with: barista node available --all",
				})
				return nil
			}
			normalized, _ := node.NormalizeVersion(version)
			requested := ""
			if match.Version != normalized {
				requested = version
				yes, _ := cmd.Flags().GetBool("yes")
				switch {
				case jsonOut || yes || !output.StdinIsTerminal():
					if !jsonOut {
						fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("node %s is not available; installing best match %s", version, match.Version)))
					}
				default:
					fmt.Fprintf(os.Stderr, "node %s is not available; install best match %s? [Y/n] ", p.Cyan(version), p.Cyan(match.Version))
					line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					if a := strings.TrimSpace(line); strings.EqualFold(a, "n") || strings.EqualFold(a, "no") {
						fmt.Println(p.Dim("aborted"))
						res.Status = output.StatusSkipped
						res.Detail = map[string]any{"reason": "aborted", "requestedVersion": requested, "version": match.Version}
						finish(cmd, []output.Result{res})
						return nil
					}
				}
			}
			suffix, e := node.PlatformSuffix(runtime.GOOS, runtime.GOARCH)
			if e != nil {
				failRes(e)
				return nil
			}
			showProgress := !jsonOut && output.StderrIsTerminal()
			mirrorBase, mirrorRaw, ok := resolveMirrorBase(cmd)
			if !ok {
				return nil
			}
			downloadURL := node.DownloadURL(node.DistBase, match.Version, suffix)
			fallbackURL := ""
			if mirrorBase != "" {
				fallbackURL = downloadURL
				downloadURL = node.MirrorDownloadURL(downloadURL, mirrorBase)
			}
			if !jsonOut {
				fmt.Fprintln(os.Stderr, "downloading "+downloadURL)
			}
			attempts, _ := cmd.Flags().GetInt("attempts")
			start := time.Now()
			result, e := node.InstallResolved(cmd.Context(), reg, regPath, match, name, downloadURL, fallbackURL, &download.Options{
				Attempts: attempts,
				OnProgress: func(received, total int64) {
					if showProgress {
						fmt.Fprint(os.Stderr, "\r"+output.ProgressLine(p, received, total, time.Since(start)))
					}
				},
				OnRetry: func(attempt int, err error) {
					if showProgress {
						fmt.Fprintln(os.Stderr)
					}
					fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("download failed: %v; retrying (attempt %d/%d)", err, attempt, attempts)))
				},
				OnFallback: func(fallbackURL string, err error) {
					if showProgress {
						fmt.Fprintln(os.Stderr)
					}
					if !jsonOut {
						fmt.Fprintln(os.Stderr, p.Yellow("mirror unavailable, falling back to "+fallbackURL))
					}
					start = time.Now()
				},
			})
			if showProgress {
				fmt.Fprintln(os.Stderr)
			}
			if e != nil {
				failRes(e)
				return nil
			}
			res.Name = result.Entry.Name
			res.Path = filepath.ToSlash(result.Entry.Path)
			res.Status = output.StatusOK
			res.Detail = map[string]any{
				"version":     result.Entry.Version,
				"managed":     true,
				"downloadUrl": result.DownloadURL,
			}
			if mirrorRaw != "" {
				res.Detail["mirror"] = mirrorRaw
			}
			if requested != "" {
				res.Detail["requestedVersion"] = requested
			}
			if !jsonOut {
				fmt.Printf("installed %s (%s) at %s\n", p.Cyan(result.Entry.Name), result.Entry.Version, filepath.ToSlash(result.Entry.Path))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().String("name", "", "register under this name (default: node-<version>)")
	cmd.Flags().Bool("yes", false, "install the best match without asking when the requested version is not available")
	cmd.Flags().Int("attempts", download.DefaultAttempts, "number of download attempts on transient failures")
	cmd.Flags().String("mirror", "", "download mirror: official, a preset name (cn|tuna|huawei), or an https:// base URL; overrides config")
	_ = cmd.RegisterFlagCompletionFunc("mirror", comp.Fn(comp.MirrorNames))
	return cmd
}
