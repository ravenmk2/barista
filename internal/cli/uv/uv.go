package uvcli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/download"
	"barista/internal/output"
	"barista/internal/uv"
	"barista/internal/workspace"
)

var exitCode *int

func NewCmd(exit *int) *cobra.Command {
	exitCode = exit
	cmd := &cobra.Command{
		Use:   "uv",
		Short: "Install uv (fast Python package manager) into ~/.local/bin",
	}
	cmd.AddCommand(installCmd())
	return cmd
}

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Download the latest uv release (sha256-verified) and install uv/uvx (and uvw on windows) into ~/.local/bin",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			if n, _ := cmd.Flags().GetInt("attempts"); n < 1 {
				return fmt.Errorf("invalid --attempts %d (want >= 1)", n)
			}
			source, _ := cmd.Flags().GetString("source")
			if err := uv.ValidateSource(source); err != nil {
				return err
			}
			version, _ := cmd.Flags().GetString("version")
			if version == "" && source != "" {
				if _, named := uv.Sources[source]; !named {
					return fmt.Errorf("--version is required with a custom --source base URL")
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			*exitCode = 0
			jsonOut, _ := cmd.Flags().GetBool("json")
			p := palette()

			if _, ok := uv.PlatformFor(runtime.GOOS, runtime.GOARCH); !ok {
				failResult(cmd, p, &output.ErrInfo{
					Code:    output.CodeUvUnsupportedPlatform,
					Message: fmt.Sprintf("no uv release for %s/%s", runtime.GOOS, runtime.GOARCH),
				})
				return nil
			}
			dir, err := uv.LocalBinDir()
			if err != nil {
				fail(cmd, p, &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()})
				return nil
			}
			target := filepath.Join(dir, uv.BinaryName())

			if found, err := exec.LookPath("uv"); err == nil && !samePath(found, target) {
				ver, _ := uv.ProbeVersion(found)
				label := "uv"
				if ver != "" {
					label = "uv " + ver
				}
				if yes, _ := cmd.Flags().GetBool("yes"); !yes {
					if !output.StdinIsTerminal() {
						fail(cmd, p, &output.ErrInfo{
							Code:    output.CodeConfirmationRequired,
							Message: fmt.Sprintf("%s is already available at %s", label, filepath.ToSlash(found)),
							Hint:    "pass --yes to install into " + filepath.ToSlash(dir) + " anyway",
						})
						return nil
					}
					fmt.Fprintf(os.Stderr, "%s is already available at %s; install into %s anyway? [y/N] ",
						p.Cyan(label), filepath.ToSlash(found), filepath.ToSlash(dir))
					line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
					a := strings.TrimSpace(line)
					if !strings.EqualFold(a, "y") && !strings.EqualFold(a, "yes") {
						emit(cmd, p, output.Result{
							Name:   "uv",
							Path:   filepath.ToSlash(found),
							Status: output.StatusSkipped,
							Action: "install",
							Detail: map[string]any{"reason": "already available", "version": ver},
						})
						return nil
					}
				}
			}

			existingVersion, _ := uv.ProbeVersion(target)
			version, _ := cmd.Flags().GetString("version")
			sourceRaw, _ := cmd.Flags().GetString("source")
			base, isGithub, err := uv.SourceBase(sourceRaw)
			if err != nil {
				fail(cmd, p, &output.ErrInfo{Code: output.CodeUsageError, Message: err.Error()})
				return nil
			}
			if version == "" && !isGithub {
				if !jsonOut {
					fmt.Fprintln(os.Stderr, p.Dim("resolving the latest uv version..."))
				}
				v, e := uv.ResolveLatest(cmd.Context(), uv.AstralInstallerURL)
				if e != nil {
					failResult(cmd, p, e)
					return nil
				}
				version = v
			}
			res := output.Result{
				Name:   "uv",
				Path:   filepath.ToSlash(target),
				Action: "install",
				Detail: map[string]any{"version": version},
			}
			if version != "" && existingVersion == version {
				res.Status = output.StatusSkipped
				res.Detail["reason"] = "already installed"
				if !jsonOut {
					fmt.Printf("%s uv %s (%s)\n", p.Dim("up to date:"), version, p.Dim(filepath.ToSlash(target)))
				}
				emit(cmd, p, res)
				return nil
			}

			assetPlatform, _ := uv.PlatformFor(runtime.GOOS, runtime.GOARCH)
			url := uv.AssetURL(base, version, assetPlatform)
			if !jsonOut {
				fmt.Fprintln(os.Stderr, "downloading "+url)
			}
			showProgress := !jsonOut && output.StderrIsTerminal()
			attempts, _ := cmd.Flags().GetInt("attempts")
			start := time.Now()
			installRes, e := uv.Install(cmd.Context(), base, version, dir, existingVersion, uv.ProbeVersion, &download.Options{
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
			})
			if showProgress {
				fmt.Fprintln(os.Stderr)
			}
			if e != nil {
				failResult(cmd, p, e)
				return nil
			}

			result := output.Result{
				Name:   "uv",
				Path:   filepath.ToSlash(target),
				Action: "install",
				Detail: map[string]any{
					"version":     installRes.Version,
					"downloadUrl": installRes.DownloadURL,
					"executables": installRes.Executables,
					"source":      sourceName(sourceRaw),
				},
			}
			if installRes.SameVersion {
				result.Status = output.StatusSkipped
				result.Detail["reason"] = "already installed"
			} else {
				result.Status = output.StatusOK
			}
			if !jsonOut {
				if installRes.SameVersion {
					fmt.Printf("%s uv %s (%s)\n", p.Dim("up to date:"), installRes.Version, p.Dim(filepath.ToSlash(target)))
				} else {
					fmt.Printf("%s uv %s at %s\n", p.Green("installed:"), p.Cyan(installRes.Version), filepath.ToSlash(target))
					if !uv.InPATH(dir) {
						fmt.Fprintln(os.Stderr, p.Yellow("note: ")+filepath.ToSlash(dir)+" is not on PATH; "+pathHint())
					}
					fmt.Fprintln(os.Stderr, p.Dim("next: uv python install 3.14  # install a CPython interpreter managed by uv"))
				}
			}
			emit(cmd, p, result)
			return nil
		},
	}
	cmd.Flags().String("version", "", "uv release version to install (default: latest)")
	cmd.Flags().String("source", "", "download source: astral (official CDN, default), github, or an https:// base URL (requires --version)")
	cmd.Flags().Bool("yes", false, "install without asking when uv is already available elsewhere")
	cmd.Flags().Int("attempts", download.DefaultAttempts, "number of download attempts on transient failures")
	_ = cmd.RegisterFlagCompletionFunc("source", comp.Fn(comp.UvSources))
	return cmd
}

func sourceName(raw string) string {
	if raw == "" {
		return "astral"
	}
	return raw
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathHint() string {
	if runtime.GOOS == "windows" {
		return `add it in PowerShell with: [Environment]::SetEnvironmentVariable("Path", $env:Path + ";$HOME\.local\bin", "User")`
	}
	return `add it with: export PATH="$HOME/.local/bin:$PATH" (persist in your shell rc file)`
}

func emit(cmd *cobra.Command, p output.Palette, res output.Result) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.NewEnvelope(commandName(cmd), "", []output.Result{res}))
		return
	}
	if res.Status == output.StatusSkipped {
		if res.Detail["reason"] == "already available" {
			fmt.Printf("%s uv at %s\n", p.Dim("skipped:"), p.Dim(res.Path))
		}
	}
}

func failResult(cmd *cobra.Command, p output.Palette, e *output.ErrInfo) {
	fmt.Fprintf(os.Stderr, "%s %s\n", p.Red("barista: "+e.Code+":"), e.Message)
	if e.Hint != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", p.Yellow("hint:"), e.Hint)
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_ = output.WriteJSON(os.Stdout, output.ErrorEnvelope(commandName(cmd), e))
	}
	*exitCode = 1
}

func fail(cmd *cobra.Command, p output.Palette, e *output.ErrInfo) {
	failResult(cmd, p, e)
	*exitCode = 2
}

func palette() output.Palette {
	cfg, err := workspace.LoadUserConfig()
	if err != nil {
		return output.NewPalette(false)
	}
	return output.NewPalette(output.ColorEnabled(cfg.Color), cfg.ColorProfile)
}

func commandName(cmd *cobra.Command) string {
	return strings.TrimPrefix(cmd.CommandPath(), "barista ")
}
