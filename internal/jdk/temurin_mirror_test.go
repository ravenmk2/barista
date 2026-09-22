package jdk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func useTemurinAPIBase(t *testing.T, url string) {
	t.Helper()
	orig := TemurinAPIBase
	TemurinAPIBase = url
	t.Cleanup(func() { TemurinAPIBase = orig })
}

func TestTemurinMirrorAsset(t *testing.T) {
	link := "https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.13%2B11/OpenJDK17U-jdk_x64_linux_hotspot_17.0.13_11.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/assets/latest/17/hotspot" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("architecture") != "x64" || q.Get("os") != "linux" || q.Get("image_type") != "jdk" || q.Get("vendor") != "eclipse" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `[{"binary":{"package":{"link":%q,"checksum":"abc123"}}}]`, link)
	}))
	defer srv.Close()
	useTemurinAPIBase(t, srv.URL)

	url, sum, err := TemurinMirrorAsset(context.Background(), "https://mirrors.tuna.tsinghua.edu.cn/Adoptium", 17, "linux", "amd64")
	if err != nil {
		t.Fatalf("TemurinMirrorAsset: %v", err)
	}
	want := "https://mirrors.tuna.tsinghua.edu.cn/Adoptium/17/jdk/x64/linux/OpenJDK17U-jdk_x64_linux_hotspot_17.0.13_11.tar.gz"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if sum != "abc123" {
		t.Errorf("sha256 = %q", sum)
	}
}

func TestTemurinMirrorAssetEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	useTemurinAPIBase(t, srv.URL)
	if _, _, err := TemurinMirrorAsset(context.Background(), "https://mirror.example.com", 99, "linux", "amd64"); err == nil {
		t.Error("want error for empty asset list")
	}
}

func TestTemurinMirrorAssetAPIFailure(t *testing.T) {
	useTemurinAPIBase(t, "http://127.0.0.1:1")
	if _, _, err := TemurinMirrorAsset(context.Background(), "https://mirror.example.com", 17, "linux", "amd64"); err == nil {
		t.Error("want error when the API is unreachable")
	}
}

func TestTemurinMirrorAssetUnsupportedPlatform(t *testing.T) {
	if _, _, err := TemurinMirrorAsset(context.Background(), "https://mirror.example.com", 17, "plan9", "amd64"); err == nil ||
		!strings.Contains(err.Error(), "unsupported OS") {
		t.Errorf("want unsupported OS error, got %v", err)
	}
}

func TestTemurinMirrorAssetEmptyChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"binary":{"package":{"link":"https://github.com/x/OpenJDK17U-jdk_x64_linux_hotspot_17.tar.gz","checksum":""}}}]`))
	}))
	defer srv.Close()
	useTemurinAPIBase(t, srv.URL)
	if _, _, err := TemurinMirrorAsset(context.Background(), "https://mirror.example.com", 17, "linux", "amd64"); err == nil {
		t.Error("want error when the API reports no checksum (mirror bytes must stay verified)")
	}
}
