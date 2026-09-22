package gradle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"barista/internal/output"
	"barista/internal/toolversion"
)

var VersionsBase = "https://services.gradle.org"

var availableHTTPClient = &http.Client{Timeout: 15 * time.Second}

type AvailableVersion struct {
	Version     string
	DownloadURL string
	ChecksumURL string
	Checksum    string
	Prerelease  bool
	Latest      bool
}

type versionsAllEntry struct {
	Version        string `json:"version"`
	Snapshot       bool   `json:"snapshot"`
	Nightly        bool   `json:"nightly"`
	ReleaseNightly bool   `json:"releaseNightly"`
	Broken         bool   `json:"broken"`
	RcFor          string `json:"rcFor"`
	MilestoneFor   string `json:"milestoneFor"`
	DownloadURL    string `json:"downloadUrl"`
	ChecksumURL    string `json:"checksumUrl"`
	Checksum       string `json:"checksum"`
	Current        bool   `json:"current"`
	Final          bool   `json:"final"`
}

// Available lists the versions published on services.gradle.org (the install
// source), ascending by version. Snapshots, nightlies and broken builds are
// filtered out; entries with rcFor/milestoneFor set or a false final flag are
// marked Prerelease; the newest final release is marked Latest.
func Available(ctx context.Context) ([]AvailableVersion, *output.ErrInfo) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, VersionsBase+"/versions/all", nil)
	if err != nil {
		return nil, availableErr(err)
	}
	resp, err := availableHTTPClient.Do(req)
	if err != nil {
		return nil, availableErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, availableErr(fmt.Errorf("GET %s/versions/all: %s", VersionsBase, resp.Status))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, availableErr(err)
	}
	var raw []versionsAllEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, availableErr(fmt.Errorf("cannot parse %s/versions/all: %v", VersionsBase, err))
	}
	var out []AvailableVersion
	for _, e := range raw {
		if e.Snapshot || e.Nightly || e.ReleaseNightly || e.Broken {
			continue
		}
		if _, _, err := toolversion.Parse(e.Version); err != nil {
			continue
		}
		out = append(out, AvailableVersion{
			Version:     e.Version,
			DownloadURL: e.DownloadURL,
			ChecksumURL: e.ChecksumURL,
			Checksum:    e.Checksum,
			Prerelease:  e.RcFor != "" || e.MilestoneFor != "" || !e.Final,
		})
	}
	if len(out) == 0 {
		return nil, availableErr(fmt.Errorf("no versions found at %s/versions/all", VersionsBase))
	}
	sort.Slice(out, func(i, j int) bool { return toolversion.Compare(out[i].Version, out[j].Version) < 0 })
	latest := -1
	for i, v := range out {
		if !v.Prerelease {
			latest = i
		}
	}
	if latest >= 0 {
		out[latest].Latest = true
	}
	return out, nil
}

// LatestPerMinor keeps only the newest version of each minor line (8.10.1 out
// of 8.10.x, 9.0.0-rc-1 out of 9.0.0-rc-*), preserving the input order.
func LatestPerMinor(versions []AvailableVersion) []AvailableVersion {
	best := map[string]int{}
	var order []string
	for i, v := range versions {
		segs, _, err := toolversion.Parse(v.Version)
		if err != nil {
			continue
		}
		key := strconv.Itoa(segs[0])
		if len(segs) > 1 {
			key = fmt.Sprintf("%d.%d", segs[0], segs[1])
		}
		if j, ok := best[key]; !ok {
			order = append(order, key)
			best[key] = i
		} else if toolversion.Compare(versions[j].Version, v.Version) < 0 {
			best[key] = i
		}
	}
	out := make([]AvailableVersion, 0, len(best))
	for _, key := range order {
		out = append(out, versions[best[key]])
	}
	return out
}

// RecentMajors keeps only the final releases belonging to the newest n major
// lines, preserving the input order.
func RecentMajors(versions []AvailableVersion, n int) []AvailableVersion {
	if n <= 0 {
		return nil
	}
	keep := map[int]bool{}
	var majors []int
	for _, v := range versions {
		if v.Prerelease {
			continue
		}
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
		majors = majors[len(majors)-n:]
		keep = map[int]bool{}
		for _, m := range majors {
			keep[m] = true
		}
	}
	var out []AvailableVersion
	for _, v := range versions {
		if v.Prerelease {
			continue
		}
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
// numeric prefix with want compete — a stable release beats a prerelease at
// the same prefix depth, then the highest version wins (8.10.1 falls back to
// 8.10.2, 8 to the latest stable 8.x, 9.1 to 9.1.0-rc-1 when no stable 9.1.x
// exists).
func MatchAvailable(versions []AvailableVersion, want string) (AvailableVersion, bool) {
	segs, _, err := toolversion.Parse(want)
	if err != nil {
		return AvailableVersion{}, false
	}
	for _, v := range versions {
		if sameVersion(v.Version, want) {
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
			(prefix == bestPrefix && versions[best].Prerelease && !v.Prerelease) ||
			(prefix == bestPrefix && versions[best].Prerelease == v.Prerelease &&
				toolversion.Compare(versions[best].Version, v.Version) < 0) {
			best, bestPrefix = i, prefix
		}
	}
	if best < 0 {
		return AvailableVersion{}, false
	}
	return versions[best], true
}

// sameVersion is a strict equality: same numeric segment count and qualifier,
// so "8.10" is not "8.10.0" (unlike toolversion.Compare, which pads missing
// segments with zeros).
func sameVersion(a, b string) bool {
	sa, qa, ea := toolversion.Parse(a)
	sb, qb, eb := toolversion.Parse(b)
	if ea != nil || eb != nil || qa != qb || len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func availableErr(err error) *output.ErrInfo {
	return &output.ErrInfo{
		Code:    output.CodeGradleAvailableFailed,
		Message: fmt.Sprintf("cannot query services.gradle.org: %v", err),
		Hint:    "check your network connection",
	}
}
