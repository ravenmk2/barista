package jdkcli

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/download"
	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

func downloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download <distro><major>",
		Short: "Download a JDK archive without installing it",
		Example: `  barista jdk download temurin17
  barista jdk download temurin21 --os linux --arch amd64 --output ~/Downloads`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if _, _, ok := jdk.ParseDistroArg(args[0]); !ok {
				return fmt.Errorf("invalid distro %q (want e.g. temurin17)", args[0])
			}
			if o, _ := cmd.Flags().GetString("os"); !validTargetOS(o) {
				return fmt.Errorf("invalid --os %q (want linux, darwin or windows)", o)
			}
			if a, _ := cmd.Flags().GetString("arch"); !validTargetArch(a) {
				return fmt.Errorf("invalid --arch %q (want amd64 or arm64)", a)
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
			distro, major, _ := jdk.ParseDistroArg(args[0])
			prov, ok := jdk.ProviderFor(distro)
			if !ok {
				failResult(cmd, p, output.Result{
					Name:   distro + strconv.Itoa(major),
					Action: "download",
					Status: output.StatusFailed,
					Error: &output.ErrInfo{
						Code:    output.CodeJDKUnsupportedDistro,
						Message: fmt.Sprintf("unsupported distro %q", distro),
						Hint:    "supported: " + strings.Join(jdk.SupportedDistros(), ", ") + "; list releases: barista jdk available",
					},
				})
				return nil
			}
			goos, _ := cmd.Flags().GetString("os")
			goarch, _ := cmd.Flags().GetString("arch")
			outFlag, _ := cmd.Flags().GetString("output")
			runDownload(cmd, p, prov, distro, major, goos, goarch, outFlag)
			return nil
		},
	}
	cmd.Flags().String("os", runtime.GOOS, "target OS (linux|darwin|windows)")
	cmd.Flags().String("arch", runtime.GOARCH, "target architecture (amd64|arm64)")
	cmd.Flags().String("output", "", "destination directory or file path (default: current directory)")
	cmd.Flags().Int("attempts", download.DefaultAttempts, "number of download attempts on transient failures")
	_ = cmd.RegisterFlagCompletionFunc("os", comp.Fn(comp.TargetOSes))
	_ = cmd.RegisterFlagCompletionFunc("arch", comp.Fn(comp.TargetArches))
	_ = cmd.RegisterFlagCompletionFunc("output", comp.Dirs)
	return cmd
}

func runDownload(cmd *cobra.Command, p output.Palette, prov jdk.Provider, distro string, major int, goos, goarch, outFlag string) {
	name := distro + strconv.Itoa(major)
	res := output.Result{Name: name, Action: "download"}
	failRes := func(e *output.ErrInfo) {
		res.Status = output.StatusFailed
		res.Error = e
		failResult(cmd, p, res)
	}
	url, err := prov.ArchiveURL(major, goos, goarch)
	if err != nil {
		failRes(&output.ErrInfo{Code: output.CodeJDKUnsupportedPlatform, Message: err.Error()})
		return
	}
	fileName, finalURL, _ := download.ResolveFileName(cmd.Context(), url)
	if fileName == "" {
		fileName = fallbackFileName(distro, major, goos, goarch, finalURL)
	}
	dest := resolveDestPath(outFlag, fileName)
	if abs, err := filepath.Abs(dest); err == nil {
		dest = abs
	}
	if _, err := os.Stat(dest); err == nil {
		failRes(&output.ErrInfo{
			Code:    output.CodeJDKExists,
			Message: fmt.Sprintf("%s already exists", filepath.ToSlash(dest)),
			Hint:    "delete the file or pass a different --output",
		})
		return
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if !jsonOut {
		fmt.Fprintln(os.Stderr, p.Dim("downloading "+url))
	}
	part := dest + ".part"
	showProgress := !jsonOut && output.StderrIsTerminal()
	attempts, _ := cmd.Flags().GetInt("attempts")
	start := time.Now()
	err = download.Download(cmd.Context(), url, part, &download.Options{
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
		failRes(&output.ErrInfo{Code: output.CodeJDKDownloadFailed, Message: err.Error()})
		return
	}
	var sum string
	if cp, ok := prov.(jdk.ChecksumProvider); ok {
		if s, ok := cp.ExpectedSHA256(major, goos, goarch); ok {
			sum = s
			if !jsonOut {
				fmt.Fprintln(os.Stderr, p.Dim("verifying sha256"))
			}
			if e := jdk.VerifySHA256(part, s); e != nil {
				_ = os.Remove(part)
				failRes(e)
				return
			}
		}
	}
	if err := os.Rename(part, dest); err != nil {
		failRes(&output.ErrInfo{Code: output.CodeJDKDownloadFailed, Message: err.Error()})
		return
	}
	fi, err := os.Stat(dest)
	if err != nil {
		failRes(&output.ErrInfo{Code: output.CodeJDKDownloadFailed, Message: err.Error()})
		return
	}
	res.Status = output.StatusOK
	res.Path = filepath.ToSlash(dest)
	res.Detail = map[string]any{
		"distro":    distro,
		"major":     major,
		"os":        goos,
		"arch":      goarch,
		"url":       url,
		"path":      filepath.ToSlash(dest),
		"sizeBytes": fi.Size(),
	}
	if sum != "" {
		res.Detail["sha256"] = sum
	}
	if !jsonOut {
		fmt.Printf("downloaded %s (%s/%s) at %s\n", p.Cyan(name), goos, goarch, filepath.ToSlash(dest))
	}
	finish(cmd, []output.Result{res})
}

func resolveDestPath(outFlag, fileName string) string {
	outFlag = workspace.ExpandHome(outFlag)
	if outFlag == "" {
		return fileName
	}
	if strings.HasSuffix(outFlag, "/") || strings.HasSuffix(outFlag, string(os.PathSeparator)) {
		return filepath.Join(outFlag, fileName)
	}
	if fi, err := os.Stat(outFlag); err == nil && fi.IsDir() {
		return filepath.Join(outFlag, fileName)
	}
	return outFlag
}

func fallbackFileName(distro string, major int, goos, goarch, finalURL string) string {
	jdkSeg := "-jdk"
	if strings.Contains(distro, "jdk") {
		jdkSeg = ""
	}
	return distro + jdkSeg + "-" + strconv.Itoa(major) + "-" + goos + "-" + goarch + archiveExt(finalURL)
}

func archiveExt(rawURL string) string {
	p := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		p = u.Path
	}
	p = strings.ToLower(p)
	switch {
	case strings.HasSuffix(p, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(p, ".tgz"):
		return ".tgz"
	case strings.HasSuffix(p, ".zip"):
		return ".zip"
	}
	return ""
}

func validTargetOS(s string) bool {
	switch s {
	case "linux", "darwin", "windows":
		return true
	}
	return false
}

func validTargetArch(s string) bool {
	switch s {
	case "amd64", "arm64":
		return true
	}
	return false
}
