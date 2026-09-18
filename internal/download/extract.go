package download

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func Extract(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot read archive: %v", err)
	}
	_ = f.Close()
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
		_ = w.Close()
		return err
	}
	return w.Close()
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
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
		_ = rc.Close()
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
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
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
