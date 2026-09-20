package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveFileNameContentDisposition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="OpenJDK21U-jdk_x64_linux_hotspot_21.0.5_11.tar.gz"`)
	}))
	defer srv.Close()
	name, finalURL, err := ResolveFileName(context.Background(), srv.URL+"/binary/latest")
	if err != nil {
		t.Fatalf("ResolveFileName: %v", err)
	}
	if name != "OpenJDK21U-jdk_x64_linux_hotspot_21.0.5_11.tar.gz" {
		t.Errorf("name = %q, want Content-Disposition filename", name)
	}
	if finalURL != srv.URL+"/binary/latest" {
		t.Errorf("finalURL = %q", finalURL)
	}
}

func TestResolveFileNameRedirectBasename(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/files/jdk-21.zip", http.StatusFound)
			return
		}
	}))
	defer srv.Close()
	name, finalURL, err := ResolveFileName(context.Background(), srv.URL+"/old")
	if err != nil {
		t.Fatalf("ResolveFileName: %v", err)
	}
	if name != "jdk-21.zip" {
		t.Errorf("name = %q, want jdk-21.zip", name)
	}
	if finalURL != srv.URL+"/files/jdk-21.zip" {
		t.Errorf("finalURL = %q, want redirected URL", finalURL)
	}
}

func TestResolveFileNameBarePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	name, _, err := ResolveFileName(context.Background(), srv.URL+"/downloads/bar.tgz")
	if err != nil {
		t.Fatalf("ResolveFileName: %v", err)
	}
	if name != "bar.tgz" {
		t.Errorf("name = %q, want bar.tgz", name)
	}
}

func TestResolveFileNameRootPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	name, _, err := ResolveFileName(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("ResolveFileName: %v", err)
	}
	if name != "" {
		t.Errorf("name = %q, want empty for root path", name)
	}
}

func TestResolveFileNameHeadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/jdk.tar.gz"
	srv.Close()
	name, finalURL, err := ResolveFileName(context.Background(), url)
	if err != nil {
		t.Fatalf("HEAD transport failure must not be an error: %v", err)
	}
	if name != "" {
		t.Errorf("name = %q, want empty on HEAD failure", name)
	}
	if finalURL != url {
		t.Errorf("finalURL = %q, want input url %q", finalURL, url)
	}
}

func TestResolveFileNameStripsPathFromContentDisposition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="../../evil.zip"`)
	}))
	defer srv.Close()
	name, _, err := ResolveFileName(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("ResolveFileName: %v", err)
	}
	if name != "evil.zip" {
		t.Errorf("name = %q, want evil.zip", name)
	}
}
