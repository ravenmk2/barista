package jdk

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Provider interface {
	ArchiveURL(major int, goos, goarch string) (string, error)
}

var providers = map[string]Provider{
	"temurin": temurinProvider{},
}

func ProviderFor(distro string) (Provider, bool) {
	p, ok := providers[distro]
	return p, ok
}

func SupportedDistros() []string {
	out := make([]string, 0, len(providers))
	for name := range providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

var distroArgRe = regexp.MustCompile(`^([a-z]+)(\d+)$`)

func ParseDistroArg(arg string) (distro string, major int, ok bool) {
	m := distroArgRe.FindStringSubmatch(arg)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return m[1], n, true
}

var TemurinAPIBase = "https://api.adoptium.net"

type temurinProvider struct{}

func (temurinProvider) ArchiveURL(major int, goos, goarch string) (string, error) {
	osName, ok := map[string]string{"linux": "linux", "darwin": "mac", "windows": "windows"}[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS %q", goos)
	}
	arch, ok := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}
	return fmt.Sprintf("%s/v3/binary/latest/%d/ga/%s/%s/jdk/hotspot/normal/eclipse", TemurinAPIBase, major, osName, arch), nil
}

func DefaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".barista", "toolchains", "jdk"), nil
}

func InstallDir(cfgValue string) (string, error) {
	if cfgValue != "" {
		return cfgValue, nil
	}
	return DefaultInstallDir()
}

type progressWriter struct {
	w        io.Writer
	received int64
	total    int64
	on       func(received, total int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	pw.received += int64(n)
	if pw.on != nil {
		pw.on(pw.received, pw.total)
	}
	return n, err
}

const DefaultDownloadAttempts = 4

type DownloadOptions struct {
	Attempts   int
	Backoff    time.Duration
	OnProgress func(received, total int64)
	OnRetry    func(attempt int, err error)
}

type permanentError struct{ msg string }

func (e *permanentError) Error() string { return e.msg }

func Download(ctx context.Context, url, dest string, opts *DownloadOptions) error {
	o := DownloadOptions{Attempts: DefaultDownloadAttempts, Backoff: 2 * time.Second}
	if opts != nil {
		if opts.Attempts > 0 {
			o.Attempts = opts.Attempts
		}
		if opts.Backoff > 0 {
			o.Backoff = opts.Backoff
		}
		o.OnProgress = opts.OnProgress
		o.OnRetry = opts.OnRetry
	}
	var err error
	for attempt := 1; attempt <= o.Attempts; attempt++ {
		err = downloadOnce(ctx, url, dest, o.OnProgress)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var perm *permanentError
		if errors.As(err, &perm) {
			return err
		}
		if attempt == o.Attempts {
			break
		}
		if o.OnRetry != nil {
			o.OnRetry(attempt+1, err)
		}
		d := o.Backoff << (attempt - 1)
		if d > 30*time.Second {
			d = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return err
}

func downloadOnce(ctx context.Context, url, dest string, onProgress func(received, total int64)) error {
	var resumed int64
	if fi, err := os.Stat(dest); err == nil {
		resumed = fi.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if resumed > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(resumed, 10)+"-")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var f *os.File
	total := resp.ContentLength
	switch {
	case resp.StatusCode == http.StatusOK:
		resumed = 0
		f, err = os.Create(dest)
	case resp.StatusCode == http.StatusPartialContent && resumed > 0:
		total += resumed
		f, err = os.OpenFile(dest, os.O_WRONLY|os.O_APPEND, 0o644)
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && resumed > 0:
		os.Remove(dest)
		return fmt.Errorf("server rejected the resume range, restarting from scratch")
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return &permanentError{msg: fmt.Sprintf("unexpected HTTP %s", resp.Status)}
	default:
		return fmt.Errorf("unexpected HTTP %s", resp.Status)
	}
	if err != nil {
		return err
	}
	pw := &progressWriter{w: f, received: resumed, total: total, on: onProgress}
	if _, err := io.Copy(pw, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func Extract(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		f.Close()
		return fmt.Errorf("cannot read archive: %v", err)
	}
	f.Close()
	switch {
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return extractTarGz(archivePath, destDir)
	case magic[0] == 'P' && magic[1] == 'K':
		return extractZip(archivePath, destDir)
	default:
		return fmt.Errorf("unknown archive format (not zip or gzip)")
	}
}

func stripFirst(name string) (string, error) {
	name = path.Clean(filepath.ToSlash(name))
	if name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
		return "", fmt.Errorf("archive entry escapes destination: %q", name)
	}
	slash := strings.Index(name, "/")
	if slash < 0 {
		return "", nil
	}
	return name[slash+1:], nil
}

func writeFile(target string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if perm == 0 {
		perm = 0o644
	}
	w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, e := range r.File {
		rel, err := stripFirst(e.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		if e.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in zip archive is unsupported: %q", e.Name)
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if e.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := e.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, rc, e.Mode().Perm())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := stripFirst(hdr.Name)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, hdr.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			resolved := filepath.Clean(filepath.Join(filepath.Dir(target), filepath.FromSlash(hdr.Linkname)))
			inside, rerr := filepath.Rel(destDir, resolved)
			if filepath.IsAbs(hdr.Linkname) || rerr != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				return fmt.Errorf("symlink %q escapes destination", hdr.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d: %q", hdr.Typeflag, hdr.Name)
		}
	}
}
