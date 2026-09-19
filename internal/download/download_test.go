package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDownload(t *testing.T) {
	body := strings.Repeat("barista", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	var lastReceived, lastTotal int64
	err := Download(context.Background(), srv.URL, dest, &Options{
		Backoff: time.Millisecond,
		OnProgress: func(received, total int64) {
			lastReceived, lastTotal = received, total
		},
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != body {
		t.Fatalf("content mismatch: %v", err)
	}
	if lastReceived != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Errorf("progress = %d/%d, want %d/%d", lastReceived, lastTotal, len(body), len(body))
	}
}

func TestDownloadHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	retries := 0
	err := Download(context.Background(), srv.URL, dest, &Options{
		Backoff: time.Millisecond,
		OnRetry: func(int, error) { retries++ },
	})
	if err == nil {
		t.Fatal("want error for 404")
	}
	if retries != 0 {
		t.Errorf("404 must not retry, got %d retries", retries)
	}
}

func TestDownloadRetryResume(t *testing.T) {
	full := strings.Repeat("barista-jdk-archive", 1000)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(full)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(full[:len(full)/2]))
			return
		}
		if !strings.HasPrefix(r.Header.Get("Range"), "bytes=") {
			t.Error("second request should carry a Range header")
		}
		var from int
		_, _ = fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from)
		w.Header().Set("Content-Length", strconv.Itoa(len(full)-from))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(full[from:]))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	var retries int
	err := Download(context.Background(), srv.URL, dest, &Options{
		Backoff: time.Millisecond,
		OnRetry: func(int, error) { retries++ },
	})
	if err != nil {
		t.Fatalf("Download with resume: %v", err)
	}
	if retries != 1 || calls != 2 {
		t.Errorf("retries = %d, calls = %d; want 1 and 2", retries, calls)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != full {
		t.Errorf("resumed content mismatch (len %d, want %d)", len(data), len(full))
	}
}

func TestDownloadRetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "jdk.bin")
	retries := 0
	err := Download(context.Background(), srv.URL, dest, &Options{
		Attempts: 3,
		Backoff:  time.Millisecond,
		OnRetry:  func(int, error) { retries++ },
	})
	if err == nil {
		t.Fatal("want error after exhausting attempts")
	}
	if retries != 2 {
		t.Errorf("retries = %d, want 2 (attempts-1)", retries)
	}
}
