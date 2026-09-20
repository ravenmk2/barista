package download

import (
	"context"
	"mime"
	"net/http"
	"path"
	"strings"
)

// ResolveFileName issues a HEAD request for rawURL (following redirects) and
// derives a file name: the Content-Disposition filename when present, else
// the basename of the final URL path ("" when neither yields one). finalURL
// is the URL after redirects. Transport failures are not errors: name is ""
// and finalURL is rawURL, so callers fall back to a synthesized name and let
// the GET surface real network errors.
func ResolveFileName(ctx context.Context, rawURL string) (name string, finalURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return "", rawURL, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", rawURL, nil
	}
	defer func() { _ = resp.Body.Close() }()
	final := resp.Request.URL.String()
	if n := baseName(contentDispositionFileName(resp.Header.Get("Content-Disposition"))); n != "" {
		return n, final, nil
	}
	return baseName(resp.Request.URL.Path), final, nil
}

func contentDispositionFileName(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

func baseName(p string) string {
	b := path.Base(strings.ReplaceAll(p, "\\", "/"))
	if b == "." || b == "/" {
		return ""
	}
	return b
}
