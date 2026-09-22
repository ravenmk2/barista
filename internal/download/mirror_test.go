package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateMirror(t *testing.T) {
	valid := []struct{ domain, value string }{
		{DomainJDK, ""},
		{DomainJDK, "official"},
		{DomainJDK, "tuna"},
		{DomainMaven, "cn"},
		{DomainMaven, "https://mirrors.example.com/apache"},
		{DomainGradle, "https://mirrors.example.com/gradle/"},
	}
	for _, tc := range valid {
		if err := ValidateMirror(tc.domain, tc.value); err != nil {
			t.Errorf("ValidateMirror(%q, %q): %v", tc.domain, tc.value, err)
		}
	}
	invalid := []struct{ domain, value string }{
		{DomainJDK, "https://mirrors.example.com/Adoptium"},
		{DomainJDK, "bogus"},
		{DomainMaven, "bogus"},
		{DomainMaven, "http://insecure.example.com/apache"},
		{DomainGradle, "ftp://mirrors.example.com/gradle"},
	}
	for _, tc := range invalid {
		if err := ValidateMirror(tc.domain, tc.value); err == nil {
			t.Errorf("ValidateMirror(%q, %q): want error", tc.domain, tc.value)
		}
	}
}

func TestValidateGlobalMirror(t *testing.T) {
	for _, v := range []string{"", "official", "cn", "tuna", "huawei", "tencent"} {
		if err := ValidateGlobalMirror(v); err != nil {
			t.Errorf("ValidateGlobalMirror(%q): %v", v, err)
		}
	}
	for _, v := range []string{"bogus", "https://mirrors.example.com/apache"} {
		if err := ValidateGlobalMirror(v); err == nil {
			t.Errorf("ValidateGlobalMirror(%q): want error", v)
		}
	}
}

func TestMirrorBase(t *testing.T) {
	cases := []struct {
		domain, value, want string
	}{
		{DomainMaven, "", ""},
		{DomainMaven, "official", ""},
		{DomainMaven, "cn", "https://mirrors.tuna.tsinghua.edu.cn/apache"},
		{DomainGradle, "cn", "https://mirrors.cloud.tencent.com/gradle"},
		{DomainJDK, "cn", "https://mirrors.tuna.tsinghua.edu.cn/Adoptium"},
		{DomainJDK, "tuna", "https://mirrors.tuna.tsinghua.edu.cn/Adoptium"},
		{DomainGradle, "tuna", ""},
		{DomainJDK, "huawei", ""},
		{DomainMaven, "huawei", "https://repo.huaweicloud.com/apache"},
		{DomainGradle, "tencent", "https://mirrors.cloud.tencent.com/gradle"},
		{DomainMaven, "https://mirrors.example.com/apache/", "https://mirrors.example.com/apache"},
		{DomainGradle, "https://mirrors.example.com/gradle", "https://mirrors.example.com/gradle"},
	}
	for _, tc := range cases {
		got, err := MirrorBase(tc.domain, tc.value)
		if err != nil || got != tc.want {
			t.Errorf("MirrorBase(%q, %q) = %q, %v; want %q", tc.domain, tc.value, got, err, tc.want)
		}
	}
	if _, err := MirrorBase(DomainMaven, "bogus"); err == nil {
		t.Error("MirrorBase(maven, bogus): want error")
	}
}

func TestWithFallbackPrimaryOK(t *testing.T) {
	fallbackHit := false
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("primary"))
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHit = true
		_, _ = w.Write([]byte("fallback"))
	}))
	defer fallback.Close()
	dest := filepath.Join(t.TempDir(), "a.bin")
	used, err := WithFallback(context.Background(), primary.URL, fallback.URL, dest, &Options{Backoff: time.Millisecond})
	if err != nil || used != primary.URL {
		t.Fatalf("WithFallback = %q, %v; want primary", used, err)
	}
	if fallbackHit {
		t.Error("fallback server must not be hit when primary succeeds")
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "primary" {
		t.Errorf("content = %q", data)
	}
}

func TestWithFallbackPrimary404(t *testing.T) {
	primary := httptest.NewServer(http.NotFoundHandler())
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			t.Error("fallback request must not resume partial bytes from the primary source")
		}
		_, _ = w.Write([]byte("fallback-content"))
	}))
	defer fallback.Close()
	dest := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(dest, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	used, err := WithFallback(context.Background(), primary.URL, fallback.URL, dest, &Options{Backoff: time.Millisecond})
	if err != nil || used != fallback.URL {
		t.Fatalf("WithFallback = %q, %v; want fallback", used, err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "fallback-content" {
		t.Errorf("content = %q (stale primary bytes must be discarded)", data)
	}
}

func TestWithFallbackBothFail(t *testing.T) {
	primary := httptest.NewServer(http.NotFoundHandler())
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fallback.Close()
	dest := filepath.Join(t.TempDir(), "a.bin")
	_, err := WithFallback(context.Background(), primary.URL, fallback.URL, dest, &Options{Attempts: 2, Backoff: time.Millisecond})
	if err == nil {
		t.Fatal("want error when both sources fail")
	}
}

func TestWithFallbackContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := filepath.Join(t.TempDir(), "a.bin")
	_, err := WithFallback(ctx, "http://127.0.0.1:1/a", "http://127.0.0.1:1/b", dest, &Options{Backoff: time.Millisecond})
	if err == nil {
		t.Fatal("want error for cancelled context")
	}
}

func TestWithFallbackCallback(t *testing.T) {
	primary := httptest.NewServer(http.NotFoundHandler())
	defer primary.Close()
	fallbackReady := false
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fallbackReady {
			t.Error("OnFallback must be invoked before the fallback download starts")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer fallback.Close()
	var gotURL string
	var gotErr error
	dest := filepath.Join(t.TempDir(), "a.bin")
	used, err := WithFallback(context.Background(), primary.URL, fallback.URL, dest, &Options{
		Backoff: time.Millisecond,
		OnFallback: func(fallbackURL string, err error) {
			fallbackReady = true
			gotURL, gotErr = fallbackURL, err
		},
	})
	if err != nil || used != fallback.URL {
		t.Fatalf("WithFallback = %q, %v", used, err)
	}
	if gotURL != fallback.URL || gotErr == nil {
		t.Errorf("OnFallback got (%q, %v), want (%q, primary error)", gotURL, gotErr, fallback.URL)
	}
}

func TestWithFallbackCallbackNotCalledOnPrimarySuccess(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer primary.Close()
	called := false
	dest := filepath.Join(t.TempDir(), "a.bin")
	_, err := WithFallback(context.Background(), primary.URL, "http://127.0.0.1:1/b", dest, &Options{
		Backoff:    time.Millisecond,
		OnFallback: func(string, error) { called = true },
	})
	if err != nil {
		t.Fatalf("WithFallback: %v", err)
	}
	if called {
		t.Error("OnFallback must not be called when the primary succeeds")
	}
}

func TestValidateMirrorBareHTTPS(t *testing.T) {
	if err := ValidateMirror(DomainMaven, "https://"); err == nil {
		t.Error("bare https:// must be rejected")
	}
}

func TestMirrorBaseTrimsAllTrailingSlashes(t *testing.T) {
	got, err := MirrorBase(DomainGradle, "https://mirrors.example.com/gradle//")
	if err != nil || got != "https://mirrors.example.com/gradle" {
		t.Errorf("MirrorBase = %q, %v", got, err)
	}
}
