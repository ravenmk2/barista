package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

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

const DefaultAttempts = 10

type Options struct {
	Attempts   int
	Backoff    time.Duration
	OnProgress func(received, total int64)
	OnRetry    func(attempt int, err error)
}

type permanentError struct{ msg string }

func (e *permanentError) Error() string { return e.msg }

func Download(ctx context.Context, url, dest string, opts *Options) error {
	o := Options{Attempts: DefaultAttempts, Backoff: 2 * time.Second}
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
	defer func() { _ = resp.Body.Close() }()
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
		_ = os.Remove(dest)
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
		_ = f.Close()
		return err
	}
	return f.Close()
}
