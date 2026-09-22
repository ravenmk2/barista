package mavencli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/download"
	"barista/internal/maven"
	"barista/internal/output"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <version>",
		Short: "Download Maven from the Apache archive (sha512-verified) into the managed install dir",
		Example: `  barista maven install 3.9.11
  barista maven install 3.9
  barista maven install 4.0.0-rc-4`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if _, _, err := maven.ParseVersion(args[0]); err != nil {
				return fmt.Errorf("invalid version %q (want e.g. 3.9.11)", args[0])
			}
			if n, _ := cmd.Flags().GetInt("attempts"); n < 1 {
				return fmt.Errorf("invalid --attempts %d (want >= 1)", n)
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
			versions, e := maven.Available(cmd.Context())
			if e != nil {
				failRes(e)
				return nil
			}
			match, found := maven.MatchAvailable(version, versions)
			if !found {
				failRes(&output.ErrInfo{
					Code:    output.CodeMavenNotFound,
					Message: fmt.Sprintf("no maven version matching %q on the Apache archive", version),
					Hint:    "list versions with: barista maven available --all",
				})
				return nil
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			if match.Version != version {
				yes, _ := cmd.Flags().GetBool("yes")
				switch {
				case jsonOut || yes || !output.StdinIsTerminal():
					if !jsonOut {
						fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("maven %s is not available; installing best match %s", version, match.Version)))
					}
				default:
					fmt.Fprintf(os.Stderr, "maven %s is not available; install best match %s? [Y/n] ", p.Cyan(version), p.Cyan(match.Version))
					line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					if a := strings.TrimSpace(line); strings.EqualFold(a, "n") || strings.EqualFold(a, "no") {
						fmt.Println(p.Dim("aborted"))
						res.Status = output.StatusSkipped
						res.Detail = map[string]any{"reason": "aborted", "requestedVersion": version, "version": match.Version}
						finish(cmd, []output.Result{res})
						return nil
					}
				}
				res.Detail = map[string]any{"requestedVersion": version}
				version = match.Version
			}
			if name == "" {
				name = autoName(reg, version)
			} else {
				res.Name = name
				if e := customNameError(name); e != nil {
					failRes(e)
					return nil
				}
				if reg.Find(name) != nil {
					failRes(&output.ErrInfo{
						Code:    output.CodeMavenExists,
						Message: fmt.Sprintf("name %q is already registered", name),
					})
					return nil
				}
			}
			res.Name = name
			url, err := maven.ArchiveURL(version)
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			root, err := maven.InstallDir(reg.InstallDir)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			destDir := filepath.Join(root, name)
			if _, err := os.Stat(destDir); err == nil {
				failRes(&output.ErrInfo{
					Code:    output.CodeMavenExists,
					Message: fmt.Sprintf("%s already exists but is not registered", filepath.ToSlash(destDir)),
					Hint:    fmt.Sprintf("delete it or register it with: barista maven add %s", filepath.ToSlash(destDir)),
				})
				return nil
			}
			if !jsonOut {
				fmt.Fprintln(os.Stderr, p.Dim("downloading "+url))
			}
			tmp, err := os.CreateTemp("", "barista-maven-*")
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenInstallFailed, Message: err.Error()})
				return nil
			}
			tmpPath := tmp.Name()
			_ = tmp.Close()
			defer func() { _ = os.Remove(tmpPath) }()
			showProgress := !jsonOut && output.StderrIsTerminal()
			attempts, _ := cmd.Flags().GetInt("attempts")
			start := time.Now()
			err = download.Download(cmd.Context(), url, tmpPath, &download.Options{
				Attempts: attempts,
				OnProgress: func(received, total int64) {
					if showProgress {
						fmt.Fprintf(os.Stderr, "\r%-100s", output.ProgressLine(received, total, time.Since(start)))
					}
				},
				OnRetry: func(attempt int, err error) {
					if showProgress {
						fmt.Fprintln(os.Stderr)
					}
					fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("download failed: %v; retrying (attempt %d/%d)", err, attempt, attempts)))
				},
			})
			if showProgress {
				fmt.Fprintln(os.Stderr)
			}
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenDownloadFailed, Message: err.Error()})
				return nil
			}
			sumTmp, err := os.CreateTemp("", "barista-maven-sha512-*")
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenInstallFailed, Message: err.Error()})
				return nil
			}
			sumPath := sumTmp.Name()
			_ = sumTmp.Close()
			defer func() { _ = os.Remove(sumPath) }()
			if err := download.Download(cmd.Context(), maven.ChecksumURL(url), sumPath, nil); err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenDownloadFailed, Message: "checksum: " + err.Error()})
				return nil
			}
			sumData, err := os.ReadFile(sumPath)
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenDownloadFailed, Message: err.Error()})
				return nil
			}
			wantSum, err := maven.ParseSHA512(string(sumData))
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenDownloadFailed, Message: err.Error()})
				return nil
			}
			if err := maven.VerifySHA512(tmpPath, wantSum); err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenChecksumMismatch, Message: err.Error()})
				return nil
			}
			if !jsonOut {
				fmt.Fprintln(os.Stderr, p.Dim("extracting to "+filepath.ToSlash(destDir)))
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenInstallFailed, Message: err.Error()})
				return nil
			}
			tmpDest, err := os.MkdirTemp(root, ".tmp-"+name+"-*")
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeMavenInstallFailed, Message: err.Error()})
				return nil
			}
			if err := download.Extract(tmpPath, tmpDest); err != nil {
				_ = os.RemoveAll(tmpDest)
				failRes(&output.ErrInfo{Code: output.CodeMavenInstallFailed, Message: err.Error()})
				return nil
			}
			if err := os.Rename(tmpDest, destDir); err != nil {
				_ = os.RemoveAll(tmpDest)
				failRes(&output.ErrInfo{Code: output.CodeMavenInstallFailed, Message: err.Error()})
				return nil
			}
			info, e := maven.Probe(destDir)
			if e == nil && info.Version != version {
				e = &output.ErrInfo{
					Code:    output.CodeMavenProbeFailed,
					Message: fmt.Sprintf("downloaded maven reports version %s, want %s", info.Version, version),
				}
			}
			if e != nil {
				_ = os.RemoveAll(destDir)
				failRes(e)
				return nil
			}
			if e := reg.Add(maven.Entry{Name: name, Version: info.Version, Path: info.Home, Managed: true}); e != nil {
				_ = os.RemoveAll(destDir)
				failRes(e)
				return nil
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Path = filepath.ToSlash(info.Home)
			if res.Detail == nil {
				res.Detail = map[string]any{}
			}
			res.Detail["version"] = info.Version
			res.Detail["managed"] = true
			if !jsonOut {
				fmt.Printf("installed %s (%s) at %s\n", p.Cyan(name), info.Version, filepath.ToSlash(info.Home))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().String("name", "", "register under this name (default: maven-<version>)")
	cmd.Flags().Bool("yes", false, "install the best match without asking when the requested version is not available")
	cmd.Flags().Int("attempts", download.DefaultAttempts, "number of download attempts on transient failures")
	return cmd
}
