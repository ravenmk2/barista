package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"barista/internal/output"
)

// ExtractEntry writes the single file entry from a release archive
// (zip or tar.gz) to dest with mode 0755. Other archive entries
// (LICENSE, completions) are ignored.
func ExtractEntry(archivePath, format, entry, dest string) *output.ErrInfo {
	fail := func(err error) *output.ErrInfo {
		return &output.ErrInfo{Code: output.CodeUpgradeExtractFailed, Message: err.Error()}
	}
	var err error
	switch format {
	case "zip":
		err = extractZipEntry(archivePath, entry, dest)
	case "tar.gz":
		err = extractTarGzEntry(archivePath, entry, dest)
	default:
		return &output.ErrInfo{
			Code:    output.CodeUpgradeExtractFailed,
			Message: fmt.Sprintf("unsupported archive format %q", format),
			Hint:    "this barista build is too old for the latest release; install it manually from GitHub releases",
		}
	}
	if err != nil {
		return fail(err)
	}
	return nil
}

func cleanEntryName(name string) string {
	return strings.TrimPrefix(path.Clean(name), "./")
}

func writeEntry(dest string, r io.Reader) error {
	w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func extractZipEntry(archivePath, entry, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if cleanEntryName(f.Name) != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeEntry(dest, rc)
		_ = rc.Close()
		return err
	}
	return fmt.Errorf("archive %s has no entry %q", archivePath, entry)
}

func extractTarGzEntry(archivePath, entry, dest string) error {
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
			return fmt.Errorf("archive %s has no entry %q", archivePath, entry)
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || cleanEntryName(hdr.Name) != entry {
			continue
		}
		return writeEntry(dest, tr)
	}
}
