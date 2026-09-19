package jdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"barista/internal/output"
)

type AvailableRelease struct {
	Major   int
	Version string
	LTS     bool
	Latest  bool
}

var availableHTTPClient = &http.Client{Timeout: 15 * time.Second}

type availableReleasesResponse struct {
	AvailableReleases    []int `json:"available_releases"`
	AvailableLTSReleases []int `json:"available_lts_releases"`
	MostRecentLTS        int   `json:"most_recent_lts"`
}

type releaseVersionsResponse struct {
	Versions []struct {
		OpenjdkVersion string `json:"openjdk_version"`
	} `json:"versions"`
}

func getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := availableHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (temurinProvider) Available(ctx context.Context) ([]AvailableRelease, *output.ErrInfo) {
	var info availableReleasesResponse
	if err := getJSON(ctx, TemurinAPIBase+"/v3/info/available_releases", &info); err != nil {
		return nil, &output.ErrInfo{
			Code:    output.CodeJDKAvailableFailed,
			Message: fmt.Sprintf("cannot query Adoptium API: %v", err),
			Hint:    "check your network connection",
		}
	}
	lts := make(map[int]bool, len(info.AvailableLTSReleases))
	for _, m := range info.AvailableLTSReleases {
		lts[m] = true
	}
	releases := make([]AvailableRelease, len(info.AvailableReleases))
	var wg sync.WaitGroup
	for i, major := range info.AvailableReleases {
		releases[i] = AvailableRelease{Major: major, LTS: lts[major], Latest: major == info.MostRecentLTS}
		wg.Add(1)
		go func(i, major int) {
			defer wg.Done()
			var rv releaseVersionsResponse
			url := fmt.Sprintf("%s/v3/info/release_versions?release_type=ga&version=%%5B%d,%d%%29&sort_order=DESC&page_size=1",
				TemurinAPIBase, major, major+1)
			if err := getJSON(ctx, url, &rv); err == nil && len(rv.Versions) > 0 {
				releases[i].Version = TrimOpenjdkVersion(rv.Versions[0].OpenjdkVersion)
			}
		}(i, major)
	}
	wg.Wait()
	sort.Slice(releases, func(i, j int) bool { return releases[i].Major < releases[j].Major })
	return releases, nil
}

// TrimOpenjdkVersion strips build metadata: "17.0.13+11" -> "17.0.13",
// "1.8.0_472-b08" -> "1.8.0_472".
func TrimOpenjdkVersion(v string) string {
	if i := strings.Index(v, "+"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "-b"); i >= 0 {
		v = v[:i]
	}
	return v
}
