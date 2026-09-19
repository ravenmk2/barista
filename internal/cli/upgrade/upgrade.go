package upgradecli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/download"
	"barista/internal/output"
	"barista/internal/upgrade"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int, version string) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade barista itself to the latest release",
		Long: "Check the latest GitHub release manifest and replace the running binary in place.\n" +
			"The download is sha256-verified against the manifest. A dev build is treated as an\n" +
			"unknown version and is always upgradeable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			exe, err := os.Executable()
			if err != nil {
				fail(cmd, &output.ErrInfo{Code: output.CodeUpgradeReplaceFailed, Message: err.Error()})
				return nil
			}
			exe, _ = filepath.EvalSymlinks(exe)
			upgrade.CleanupStale(exe)

			m, e := upgrade.FetchManifest(cmd.Context(), upgrade.DefaultBaseURL)
			if e != nil {
				failResult(cmd, e)
				return nil
			}
			newer, known := upgrade.Newer(version, m.Version)
			res := output.Result{
				Name:   "barista",
				Path:   filepath.ToSlash(exe),
				Action: "upgrade",
				Detail: map[string]any{
					"current":         version,
					"latest":          m.Version,
					"updateAvailable": newer,
				},
			}
			if !known {
				res.Detail["note"] = "current build is not a release tag"
			}
			if !newer {
				res.Status = output.StatusOK
				if known {
					res.Detail["reason"] = "already up to date"
				}
				emit(cmd, res)
				return nil
			}
			if checkOnly, _ := cmd.Flags().GetBool("check"); checkOnly {
				res.Status = output.StatusOK
				res.Detail["checkOnly"] = true
				emit(cmd, res)
				return nil
			}
			asset, ok := m.Asset(runtime.GOOS, runtime.GOARCH)
			if !ok {
				failResult(cmd, &output.ErrInfo{
					Code:    output.CodeUpgradeCheckFailed,
					Message: fmt.Sprintf("release %s has no asset for %s/%s", m.Version, runtime.GOOS, runtime.GOARCH),
				})
				return nil
			}
			if !confirmUpgrade(cmd, version, m.Version, exe) {
				res.Status = output.StatusSkipped
				res.Detail["reason"] = "aborted"
				emit(cmd, res)
				return nil
			}
			if e := downloadAndVerify(cmd, m, asset, exe); e != nil {
				failResult(cmd, e)
				return nil
			}
			res.Status = output.StatusOK
			res.Detail["file"] = asset.File
			res.Detail["size"] = asset.Size
			emit(cmd, res)
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "only check for a newer release, do not download or replace")
	cmd.Flags().Bool("yes", false, "skip the confirmation prompt")
	return cmd
}

func confirmUpgrade(cmd *cobra.Command, current, latest, exe string) bool {
	if yes, _ := cmd.Flags().GetBool("yes"); yes {
		return true
	}
	if !output.StdinIsTerminal() {
		fail(cmd, &output.ErrInfo{
			Code:    output.CodeConfirmationRequired,
			Message: fmt.Sprintf("upgrading %s → %s replaces %s", current, latest, filepath.ToSlash(exe)),
			Hint:    "pass --yes to confirm",
		})
		return false
	}
	p := output.NewPalette(output.ColorEnabled(""))
	fmt.Fprintf(os.Stderr, "upgrade %s → %s, replacing %s? [y/N] ", p.Cyan(current), p.Cyan(latest), filepath.ToSlash(exe))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")
}

func downloadAndVerify(cmd *cobra.Command, m *upgrade.Manifest, asset upgrade.Asset, exe string) *output.ErrInfo {
	url := strings.TrimRight(upgrade.DefaultBaseURL, "/") + "/" + asset.File
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".barista-upgrade-*")
	if err != nil {
		return &output.ErrInfo{Code: output.CodeUpgradeDownloadFailed, Message: err.Error()}
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	jsonOut, _ := cmd.Flags().GetBool("json")
	showProgress := !jsonOut && output.StderrIsTerminal()
	start := time.Now()
	err = download.Download(cmd.Context(), url, tmpPath, &download.Options{
		OnProgress: func(received, total int64) {
			if showProgress {
				fmt.Fprintf(os.Stderr, "\r%-100s", output.ProgressLine(received, total, time.Since(start)))
			}
		},
		OnRetry: func(attempt int, err error) {
			if showProgress {
				fmt.Fprintf(os.Stderr, "\nretry %d: %v\n", attempt, err)
			}
		},
	})
	if showProgress {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		return &output.ErrInfo{
			Code:    output.CodeUpgradeDownloadFailed,
			Message: fmt.Sprintf("cannot download %s: %v", url, err),
			Hint:    "check your network connection and try again",
		}
	}
	if e := upgrade.VerifySHA256(tmpPath, asset.SHA256); e != nil {
		return e
	}
	return upgrade.ReplaceBinary(exe, tmpPath)
}

func emit(cmd *cobra.Command, res output.Result) {
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), "", []output.Result{res}))
		return
	}
	cfg, err := workspace.LoadUserConfig()
	color := err == nil && output.ColorEnabled(cfg.Color)
	p := output.NewPalette(color)
	switch res.Status {
	case output.StatusOK:
		if res.Detail["checkOnly"] == true {
			fmt.Printf("%s %s → %s available (%s)\n", p.Green("update available:"), res.Detail["current"], p.Cyan(fmt.Sprint(res.Detail["latest"])), filepath.ToSlash(res.Path))
		} else if reason, ok := res.Detail["reason"].(string); ok {
			fmt.Printf("%s barista %s (%s)\n", p.Green("up to date:"), res.Detail["latest"], p.Dim(reason))
		} else {
			fmt.Printf("%s %s → %s\n", p.Green("upgraded"), res.Detail["current"], p.Cyan(fmt.Sprint(res.Detail["latest"])))
		}
	case output.StatusSkipped:
		fmt.Println(p.Dim("aborted"))
	}
}

func failResult(cmd *cobra.Command, e *output.ErrInfo) {
	fmt.Fprintf(os.Stderr, "barista: %s: %s\n", e.Code, e.Message)
	if e.Hint != "" {
		fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
	}
	*exitCode = 1
}

func fail(cmd *cobra.Command, e *output.ErrInfo) {
	failResult(cmd, e)
	*exitCode = 2
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
}
