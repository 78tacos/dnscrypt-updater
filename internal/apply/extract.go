package apply

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractArchive(archivePath, destDir string) error {
	name := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(name, ".zip"):
		return unzip(archivePath, destDir)
	case strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz"):
		return untarGz(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive type: %s", filepath.Base(archivePath))
	}
}

func unzip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if err := extractZipFile(destDir, f); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(destDir string, f *zip.File) error {
	target, err := safeJoin(destDir, f.Name)
	if err != nil {
		return err
	}
	if strings.HasSuffix(f.Name, "/") || f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(f.Mode()))
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(rc, maxAssetBytes))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func untarGz(archivePath, destDir string) error {
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
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(os.FileMode(hdr.Mode)))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, io.LimitReader(tr, maxAssetBytes))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			// skip links and other types
		}
	}
}

func safeJoin(destDir, name string) (string, error) {
	slashName := strings.ReplaceAll(name, `\`, `/`)
	for _, part := range strings.Split(slashName, "/") {
		if part == ".." {
			return "", fmt.Errorf("refusing archive path %q (zip slip)", name)
		}
	}
	cleaned := filepath.Clean("/" + slashName)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return destDir, nil
	}
	target := filepath.Join(destDir, cleaned)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("refusing archive path %q (zip slip)", name)
	}
	return target, nil
}

func fileMode(mode os.FileMode) os.FileMode {
	if mode == 0 {
		return 0o644
	}
	return mode
}
