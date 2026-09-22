package download

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	DomainJDK    = "jdk"
	DomainMaven  = "maven"
	DomainGradle = "gradle"
)

// MirrorPresets maps a preset name to per-domain binary download base URLs;
// a domain absent from a preset keeps the official source. The variable is
// exported so tests can inject httptest bases.
var MirrorPresets = map[string]map[string]string{
	"cn": {
		DomainJDK:    "https://mirrors.tuna.tsinghua.edu.cn/Adoptium",
		DomainMaven:  "https://mirrors.tuna.tsinghua.edu.cn/apache",
		DomainGradle: "https://mirrors.cloud.tencent.com/gradle",
	},
	"tuna": {
		DomainJDK:   "https://mirrors.tuna.tsinghua.edu.cn/Adoptium",
		DomainMaven: "https://mirrors.tuna.tsinghua.edu.cn/apache",
	},
	"huawei": {
		DomainMaven:  "https://repo.huaweicloud.com/apache",
		DomainGradle: "https://repo.huaweicloud.com/gradle",
	},
	"tencent": {
		DomainMaven:  "https://mirrors.cloud.tencent.com/apache",
		DomainGradle: "https://mirrors.cloud.tencent.com/gradle",
	},
}

func presetNames() string {
	names := make([]string, 0, len(MirrorPresets))
	for name := range MirrorPresets {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

// ValidateMirror checks a per-domain mirror value: empty and official are
// always legal, a known preset name is legal, and maven/gradle additionally
// accept a custom https:// base URL (the jdk domain does not: the temurin
// mirror layout cannot be derived from an arbitrary base).
func ValidateMirror(domain, value string) error {
	if value == "" || value == "official" {
		return nil
	}
	if _, ok := MirrorPresets[value]; ok {
		return nil
	}
	if domain != DomainJDK && len(value) > len("https://") && strings.HasPrefix(value, "https://") {
		return nil
	}
	if domain == DomainJDK {
		return fmt.Errorf("invalid %s download mirror %q (want official|%s)", domain, value, presetNames())
	}
	return fmt.Errorf("invalid %s download mirror %q (want official|%s or an https:// base URL)", domain, value, presetNames())
}

// ValidateGlobalMirror checks the global download.mirror value: only empty,
// official and preset names are legal.
func ValidateGlobalMirror(value string) error {
	if value == "" || value == "official" {
		return nil
	}
	if _, ok := MirrorPresets[value]; ok {
		return nil
	}
	return fmt.Errorf("invalid download mirror %q (want official|%s)", value, presetNames())
}

// MirrorBase resolves a configured mirror value to the download base URL for
// domain; "" means the official source (unset, official, or a preset without
// coverage for the domain). A custom base URL is returned with any trailing
// slash trimmed.
func MirrorBase(domain, value string) (string, error) {
	if err := ValidateMirror(domain, value); err != nil {
		return "", err
	}
	if value == "" || value == "official" {
		return "", nil
	}
	if preset, ok := MirrorPresets[value]; ok {
		return preset[domain], nil
	}
	return strings.TrimRight(value, "/"), nil
}

// WithFallback downloads primary to dest; on failure (other than context
// cancellation) it removes any partial dest — bytes from a different source
// must not be resumed — and retries from fallback, invoking opts.OnFallback
// before the fallback download starts. It returns the URL that succeeded, or
// the fallback error when both fail.
func WithFallback(ctx context.Context, primary, fallback, dest string, opts *Options) (usedURL string, err error) {
	if err := Download(ctx, primary, dest, opts); err == nil {
		return primary, nil
	} else if ctx.Err() != nil {
		return "", ctx.Err()
	} else {
		_ = os.Remove(dest)
		if opts != nil && opts.OnFallback != nil {
			opts.OnFallback(fallback, err)
		}
	}
	if err := Download(ctx, fallback, dest, opts); err != nil {
		return "", err
	}
	return fallback, nil
}
