package jdk

import (
	"context"
	"fmt"
	"path"
)

type temurinAssetsResponse []struct {
	Binary struct {
		Package struct {
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
		} `json:"package"`
	} `json:"binary"`
}

// TemurinMirrorAsset resolves the latest GA temurin asset for major via the
// Adoptium API and maps it onto the mirror directory layout
// <mirrorBase>/<major>/jdk/<arch>/<os>/<filename>, returning the mirror URL
// and the API-reported sha256 of the asset.
func TemurinMirrorAsset(ctx context.Context, mirrorBase string, major int, goos, goarch string) (url, sha256 string, err error) {
	osName, arch, err := temurinPlatformTokens(goos, goarch)
	if err != nil {
		return "", "", err
	}
	apiURL := fmt.Sprintf("%s/v3/assets/latest/%d/hotspot?architecture=%s&image_type=jdk&os=%s&vendor=eclipse",
		TemurinAPIBase, major, arch, osName)
	var assets temurinAssetsResponse
	if err := getJSON(ctx, apiURL, &assets); err != nil {
		return "", "", err
	}
	if len(assets) == 0 || assets[0].Binary.Package.Link == "" {
		return "", "", fmt.Errorf("no temurin %d asset for %s/%s", major, osName, arch)
	}
	if assets[0].Binary.Package.Checksum == "" {
		return "", "", fmt.Errorf("no checksum reported for temurin %d asset %s/%s", major, osName, arch)
	}
	link := assets[0].Binary.Package.Link
	return fmt.Sprintf("%s/%d/jdk/%s/%s/%s", mirrorBase, major, arch, osName, path.Base(link)),
		assets[0].Binary.Package.Checksum, nil
}
