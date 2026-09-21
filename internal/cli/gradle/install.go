package gradlecli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/download"
	"barista/internal/gradle"
	"barista/internal/output"
	"barista/internal/toolversion"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <version>",
		Short: "Download Gradle (bin distribution, sha256-verified) from services.gradle.org into the managed install dir",
		Example: `  barista gradle install 8.10.2
	  barista gradle install 8
	  barista gradle install 9.0.0-rc-1`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if _, _, err := toolversion.Parse(args[0]); err != nil {
				return fmt.Errorf("invalid version %q (want e.g. 8.10.2)", args[0])
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
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			showProgress := !jsonOut && output.StderrIsTerminal()
			start := time.Now()
			result, e := gradle.Install(cmd.Context(), reg, regPath, version, name, &download.Options{
				OnProgress: func(received, total int64) {
					if showProgress {
						fmt.Fprintf(os.Stderr, "\r%-100s", output.ProgressLine(received, total, time.Since(start)))
					}
				},
				OnRetry: func(attempt int, err error) {
					if showProgress {
						fmt.Fprintln(os.Stderr)
					}
					fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("download failed: %v; retrying (%d/%d)", err, attempt, download.DefaultAttempts)))
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
				"version": result.Entry.Version,
				"managed": true,
			}
			if result.RequestedVersion != "" {
				if !jsonOut {
					fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("gradle %s is not available; installed best match %s", result.RequestedVersion, result.Entry.Version)))
				}
				res.Detail["requestedVersion"] = result.RequestedVersion
			}
			if !jsonOut {
				fmt.Printf("installed %s (%s) at %s\n", p.Cyan(result.Entry.Name), result.Entry.Version, filepath.ToSlash(result.Entry.Path))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	cmd.Flags().String("name", "", "register under this name (default: gradle-<version>)")
	return cmd
}
