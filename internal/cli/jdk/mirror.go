package jdkcli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"barista/internal/cli/comp"
	"barista/internal/download"
	"barista/internal/jdk"
	"barista/internal/output"
	"barista/internal/workspace"
)

func configErrInfo(err error) *output.ErrInfo {
	e := &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	var le *workspace.LoadError
	if errors.As(err, &le) {
		e.Code, e.Message, e.Hint = le.Code, le.Message, le.Hint
	}
	return e
}

func addMirrorFlag(cmd *cobra.Command) {
	cmd.Flags().String("mirror", "", "download mirror for temurin: official or a preset name (cn|tuna|huawei|tencent); overrides config")
	_ = cmd.RegisterFlagCompletionFunc("mirror", comp.Fn(comp.MirrorNames))
}

func validateMirrorFlag(cmd *cobra.Command) error {
	if !cmd.Flags().Changed("mirror") {
		return nil
	}
	v, _ := cmd.Flags().GetString("mirror")
	return download.ValidateMirror(download.DomainJDK, v)
}

// resolveTemurinMirror resolves the configured jdk download mirror for
// temurin into a concrete mirror URL plus the Adoptium-API-reported sha256.
// An explicit --mirror flag wins over config (official disables mirroring).
// Non-temurin distros and an unset/official mirror yield empty results.
// A config error is a hard failure; an asset resolution failure degrades to
// a soft warning and the caller proceeds with the official URL.
func resolveTemurinMirror(ctx context.Context, cmd *cobra.Command, distro string, major int, goos, goarch string) (mirrorURL, sha256, raw, warn string, e *output.ErrInfo) {
	if distro != "temurin" {
		return "", "", "", "", nil
	}
	if cmd.Flags().Changed("mirror") {
		raw, _ = cmd.Flags().GetString("mirror")
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", "", "", &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
		}
		cfg, err := workspace.LoadMergedConfig(cwd)
		if err != nil {
			return "", "", "", "", configErrInfo(err)
		}
		raw = cfg.MirrorValueFor(download.DomainJDK)
	}
	base, err := download.MirrorBase(download.DomainJDK, raw)
	if err != nil {
		return "", "", "", "", &output.ErrInfo{Code: output.CodeConfigError, Message: err.Error()}
	}
	if base == "" {
		return "", "", "", "", nil
	}
	u, sum, err := jdk.TemurinMirrorAsset(ctx, base, major, goos, goarch)
	if err != nil {
		return "", "", "", fmt.Sprintf("cannot resolve mirror asset (%v); using official source", err), nil
	}
	return u, sum, raw, "", nil
}
