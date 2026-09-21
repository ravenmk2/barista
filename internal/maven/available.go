package maven

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"barista/internal/output"
)

type AvailableVersion struct {
	Version string
	Latest  bool
}

var availableHTTPClient = &http.Client{Timeout: 15 * time.Second}

var (
	majorDirRe   = regexp.MustCompile(`href="maven-(\d+)/"`)
	versionDirRe = regexp.MustCompile(`href="([0-9][0-9A-Za-z.-]*)/"`)
)

// Available lists the versions published on the Apache archive (the install
// source), ascending by version. The newest overall is marked Latest.
func Available(ctx context.Context) ([]AvailableVersion, *output.ErrInfo) {
	index, err := fetchListing(ctx, ArchiveBase+"/")
	if err != nil {
		return nil, availableErr(err)
	}
	majors := majorDirRe.FindAllStringSubmatch(index, -1)
	if len(majors) == 0 {
		return nil, availableErr(fmt.Errorf("no maven-<major> directories found at %s", ArchiveBase))
	}
	lines := make([][]string, len(majors))
	var wg sync.WaitGroup
	for i, m := range majors {
		wg.Add(1)
		go func(i int, major string) {
			defer wg.Done()
			listing, err := fetchListing(ctx, fmt.Sprintf("%s/maven-%s/", ArchiveBase, major))
			if err != nil {
				return
			}
			for _, v := range versionDirRe.FindAllStringSubmatch(listing, -1) {
				if _, _, err := ParseVersion(v[1]); err == nil {
					lines[i] = append(lines[i], v[1])
				}
			}
		}(i, m[1])
	}
	wg.Wait()
	var versions []string
	for _, l := range lines {
		versions = append(versions, l...)
	}
	if len(versions) == 0 {
		return nil, availableErr(fmt.Errorf("no versions found under %s", ArchiveBase))
	}
	sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) < 0 })
	out := make([]AvailableVersion, len(versions))
	for i, v := range versions {
		out[i] = AvailableVersion{Version: v, Latest: i == len(versions)-1}
	}
	return out, nil
}

// LatestPerMinor keeps only the newest version of each minor line (3.8.9 out
// of 3.8.x, 4.0.0-rc-4 out of 4.0.0-rc-*), preserving the input order.
func LatestPerMinor(versions []AvailableVersion) []AvailableVersion {
	best := map[string]int{}
	var order []string
	for i, v := range versions {
		segs, _, err := ParseVersion(v.Version)
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
		} else if CompareVersions(versions[j].Version, v.Version) < 0 {
			best[key] = i
		}
	}
	out := make([]AvailableVersion, 0, len(best))
	for _, key := range order {
		out = append(out, versions[best[key]])
	}
	return out
}

func availableErr(err error) *output.ErrInfo {
	return &output.ErrInfo{
		Code:    output.CodeMavenAvailableFailed,
		Message: fmt.Sprintf("cannot query the Apache archive: %v", err),
		Hint:    "check your network connection",
	}
}

// MatchAvailable picks the best match for want among available versions: an
// exact version match wins, otherwise the highest version sharing the longest
// numeric prefix with want (4.0.0-rc-4 falls back to 4.0.0-rc-6, 3.9 to the
// latest 3.9.x).
func MatchAvailable(want string, versions []AvailableVersion) (AvailableVersion, bool) {
	segs, _, err := ParseVersion(want)
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
		vs, _, err := ParseVersion(v.Version)
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
		if prefix > bestPrefix || (prefix == bestPrefix && CompareVersions(versions[best].Version, v.Version) < 0) {
			best, bestPrefix = i, prefix
		}
	}
	if best < 0 {
		return AvailableVersion{}, false
	}
	return versions[best], true
}

// sameVersion is a strict equality: same numeric segment count and qualifier,
// so "3.9" is not "3.9.0" (unlike CompareVersions, which pads missing
// segments with zeros).
func sameVersion(a, b string) bool {
	sa, qa, ea := ParseVersion(a)
	sb, qb, eb := ParseVersion(b)
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

func fetchListing(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := availableHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
