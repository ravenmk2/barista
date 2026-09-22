package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"barista/internal/output"
	"barista/internal/toolversion"
)

// DistBase is the nodejs.org dist base URL; mirrors serve the same layout.
// The variable is exported so tests can inject an httptest base.
var DistBase = "https://nodejs.org/dist"

// MirrorDownloadURL rewrites an official dist URL onto a mirror base URL (the
// mirror serves the same dist layout); URLs not under DistBase are returned
// unchanged.
func MirrorDownloadURL(downloadURL, mirrorBase string) string {
	if mirrorBase == "" || !strings.HasPrefix(downloadURL, DistBase) {
		return downloadURL
	}
	return mirrorBase + strings.TrimPrefix(downloadURL, DistBase)
}

var availableHTTPClient = &http.Client{Timeout: 15 * time.Second}

type AvailableVersion struct {
	Version string
	LTS     string
	Latest  bool
}

type indexEntry struct {
	Version string `json:"version"`
	LTS     any    `json:"lts"`
}

// Available lists the versions published on the dist server (the install
// source), descending by version. Entries whose version does not parse are
// skipped; LTS carries the codename ("Jod") for LTS lines; the newest version
// is marked Latest.
func Available(ctx context.Context) ([]AvailableVersion, *output.ErrInfo) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DistBase+"/index.json", nil)
	if err != nil {
		return nil, availableErr(err)
	}
	resp, err := availableHTTPClient.Do(req)
	if err != nil {
		return nil, availableErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, availableErr(fmt.Errorf("GET %s/index.json: %s", DistBase, resp.Status))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, availableErr(err)
	}
	var raw []indexEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, availableErr(fmt.Errorf("cannot parse %s/index.json: %v", DistBase, err))
	}
	var out []AvailableVersion
	for _, e := range raw {
		v, err := NormalizeVersion(e.Version)
		if err != nil {
			continue
		}
		av := AvailableVersion{Version: v}
		if codename, ok := e.LTS.(string); ok {
			av.LTS = codename
		}
		out = append(out, av)
	}
	if len(out) == 0 {
		return nil, availableErr(fmt.Errorf("no versions found at %s/index.json", DistBase))
	}
	sort.Slice(out, func(i, j int) bool { return toolversion.Compare(out[i].Version, out[j].Version) > 0 })
	out[0].Latest = true
	return out, nil
}

// LatestPerMajor keeps only the newest version of each major line, preserving
// the input order.
func LatestPerMajor(versions []AvailableVersion) []AvailableVersion {
	best := map[int]int{}
	var order []int
	for i, v := range versions {
		segs, _, err := toolversion.Parse(v.Version)
		if err != nil {
			continue
		}
		major := segs[0]
		if j, ok := best[major]; !ok {
			order = append(order, major)
			best[major] = i
		} else if toolversion.Compare(versions[j].Version, v.Version) < 0 {
			best[major] = i
		}
	}
	out := make([]AvailableVersion, 0, len(best))
	for _, major := range order {
		out = append(out, versions[best[major]])
	}
	return out
}

// RecentMajors keeps only the versions belonging to the newest n major lines,
// preserving the input order.
func RecentMajors(versions []AvailableVersion, n int) []AvailableVersion {
	if n <= 0 {
		return nil
	}
	keep := map[int]bool{}
	var majors []int
	for _, v := range versions {
		segs, _, err := toolversion.Parse(v.Version)
		if err != nil {
			continue
		}
		if !keep[segs[0]] {
			keep[segs[0]] = true
			majors = append(majors, segs[0])
		}
	}
	if len(majors) > n {
		keep = map[int]bool{}
		for _, m := range majors[:n] {
			keep[m] = true
		}
	}
	var out []AvailableVersion
	for _, v := range versions {
		segs, _, err := toolversion.Parse(v.Version)
		if err != nil || !keep[segs[0]] {
			continue
		}
		out = append(out, v)
	}
	return out
}

// MatchAvailable picks the best match for want among available versions: an
// exact version match wins, otherwise the candidates sharing the longest
// numeric prefix with want compete and the highest version wins (22.14 falls
// back to 22.14.0, 22 to the latest 22.x).
func MatchAvailable(versions []AvailableVersion, want string) (AvailableVersion, bool) {
	want = strings.TrimPrefix(strings.TrimSpace(want), "v")
	segs, _, err := toolversion.Parse(want)
	if err != nil {
		return AvailableVersion{}, false
	}
	for _, v := range versions {
		if v.Version == want {
			return v, true
		}
	}
	best, bestPrefix := -1, 0
	for i, v := range versions {
		vs, _, err := toolversion.Parse(v.Version)
		if err != nil {
			continue
		}
		prefix := 0
		for prefix < len(segs) && prefix < len(vs) && segs[prefix] == vs[prefix] {
			prefix++
		}
		if prefix == 0 {
			continue
		}
		if prefix > bestPrefix ||
			(prefix == bestPrefix && toolversion.Compare(versions[best].Version, v.Version) < 0) {
			best, bestPrefix = i, prefix
		}
	}
	if best < 0 {
		return AvailableVersion{}, false
	}
	return versions[best], true
}

func availableErr(err error) *output.ErrInfo {
	return &output.ErrInfo{
		Code:    output.CodeNodeAvailableFailed,
		Message: fmt.Sprintf("cannot query nodejs.org: %v", err),
		Hint:    "check your network connection",
	}
}
