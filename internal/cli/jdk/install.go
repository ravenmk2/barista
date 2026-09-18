package jdkcli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/jdk"
	"barista/internal/output"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <distro><major>",
		Short: "Download and install a JDK into the managed install dir",
		Example: `  barista jdk install temurin17
  barista jdk install temurin8`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if _, _, ok := jdk.ParseDistroArg(args[0]); !ok {
				return fmt.Errorf("invalid distro %q (want e.g. temurin17)", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			*exitCode = 0
			cfg, p, ok := userSettings(cmd)
			if !ok {
				return nil
			}
			reg, regPath, ok := loadRegistry(cmd)
			if !ok {
				return nil
			}
			distro, major, _ := jdk.ParseDistroArg(args[0])
			name := distro + strconv.Itoa(major)
			res := output.Result{Name: name, Action: "install"}
			failRes := func(e *output.ErrInfo) {
				res.Status = output.StatusFailed
				res.Error = e
				failResult(cmd, p, res)
			}
			prov, ok := jdk.ProviderFor(distro)
			if !ok {
				failRes(&output.ErrInfo{
					Code:    output.CodeJDKUnsupportedDistro,
					Message: fmt.Sprintf("unsupported distro %q", distro),
					Hint:    "supported: " + strings.Join(jdk.SupportedDistros(), ", "),
				})
				return nil
			}
			if reg.Find(name) != nil {
				failRes(&output.ErrInfo{
					Code:    output.CodeJDKExists,
					Message: fmt.Sprintf("JDK %q is already registered", name),
					Hint:    "to reinstall, run: barista jdk uninstall " + name,
				})
				return nil
			}
			root, err := jdk.InstallDir(cfg.InstallDir)
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			url, err := prov.ArchiveURL(major, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeJDKUnsupportedPlatform, Message: err.Error()})
				return nil
			}
			destDir := filepath.Join(root, name)
			if _, err := os.Stat(destDir); err == nil {
				failRes(&output.ErrInfo{
					Code:    output.CodeJDKExists,
					Message: fmt.Sprintf("%s already exists but is not registered", filepath.ToSlash(destDir)),
					Hint:    fmt.Sprintf("delete it or register it with: barista jdk add %s %s", name, filepath.ToSlash(destDir)),
				})
				return nil
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			if !jsonOut {
				fmt.Fprintln(os.Stderr, p.Dim("downloading "+url))
			}
			tmp, err := os.CreateTemp("", "barista-jdk-*")
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeJDKInstallFailed, Message: err.Error()})
				return nil
			}
			tmpPath := tmp.Name()
			tmp.Close()
			defer os.Remove(tmpPath)
			showProgress := !jsonOut && output.StderrIsTerminal()
			start := time.Now()
			err = jdk.Download(cmd.Context(), url, tmpPath, &jdk.DownloadOptions{
				OnProgress: func(received, total int64) {
					if showProgress {
						fmt.Fprintf(os.Stderr, "\r%-100s", progressLine(received, total, time.Since(start)))
					}
				},
				OnRetry: func(attempt int, err error) {
					if showProgress {
						fmt.Fprintln(os.Stderr)
					}
					fmt.Fprintln(os.Stderr, p.Yellow(fmt.Sprintf("download failed: %v; retrying (%d/%d)", err, attempt, jdk.DefaultDownloadAttempts)))
				},
			})
			if showProgress {
				fmt.Fprintln(os.Stderr)
			}
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeJDKDownloadFailed, Message: err.Error()})
				return nil
			}
			if !jsonOut {
				fmt.Fprintln(os.Stderr, p.Dim("extracting to "+filepath.ToSlash(destDir)))
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				failRes(&output.ErrInfo{Code: output.CodeJDKInstallFailed, Message: err.Error()})
				return nil
			}
			tmpDest, err := os.MkdirTemp(root, ".tmp-"+name+"-*")
			if err != nil {
				failRes(&output.ErrInfo{Code: output.CodeJDKInstallFailed, Message: err.Error()})
				return nil
			}
			if err := jdk.Extract(tmpPath, tmpDest); err != nil {
				os.RemoveAll(tmpDest)
				failRes(&output.ErrInfo{Code: output.CodeJDKInstallFailed, Message: err.Error()})
				return nil
			}
			if err := os.Rename(tmpDest, destDir); err != nil {
				os.RemoveAll(tmpDest)
				failRes(&output.ErrInfo{Code: output.CodeJDKInstallFailed, Message: err.Error()})
				return nil
			}
			info, e := jdk.Probe(destDir)
			if e == nil && info.Major != major {
				e = &output.ErrInfo{
					Code:    output.CodeJDKProbeFailed,
					Message: fmt.Sprintf("downloaded JDK reports major %d, want %d", info.Major, major),
				}
			}
			if e != nil {
				os.RemoveAll(destDir)
				failRes(e)
				return nil
			}
			if e := reg.Add(jdk.Entry{Name: name, Major: info.Major, Version: info.Version, Path: info.Home, Managed: true}); e != nil {
				os.RemoveAll(destDir)
				failRes(e)
				return nil
			}
			if !saveRegistry(cmd, reg, regPath) {
				return nil
			}
			res.Status = output.StatusOK
			res.Path = filepath.ToSlash(info.Home)
			res.Detail = map[string]any{
				"major":   info.Major,
				"version": info.Version,
				"distro":  info.Distro,
				"managed": true,
			}
			if !jsonOut {
				fmt.Printf("installed %s (%s %s) at %s\n", p.Cyan(name), info.Distro, info.Version, filepath.ToSlash(info.Home))
			}
			finish(cmd, []output.Result{res})
			return nil
		},
	}
	return cmd
}

func progressLine(received, total int64, elapsed time.Duration) string {
	const width = 25
	rate := float64(received) / max(elapsed.Seconds(), 0.5)
	speed := humanBytes(int64(rate)) + "/s"
	if total <= 0 {
		return fmt.Sprintf("downloading %s (%s)", humanBytes(received), speed)
	}
	pct := float64(received) / float64(total)
	bar := "[" + strings.Repeat("=", int(pct*width)) + strings.Repeat("-", width-int(pct*width)) + "]"
	line := fmt.Sprintf("downloading %s %5.1f%% %s/%s at %s", bar, pct*100, humanBytes(received), humanBytes(total), speed)
	if rate > 0 {
		eta := time.Duration(float64(total-received) / rate * float64(time.Second))
		line += fmt.Sprintf(", eta %s", eta.Round(time.Second))
	}
	return line
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/float64(int64(1)<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/float64(int64(1)<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/float64(int64(1)<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
