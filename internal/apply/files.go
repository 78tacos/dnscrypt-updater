package apply

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func findProxyBinary(root, goos string) (string, error) {
	want := proxyBinaryName(goos)
	var found []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(info.Name(), want) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", fmt.Errorf("archive does not contain %s", want)
	}
	best := found[0]
	bestDepth := strings.Count(best, string(os.PathSeparator))
	for _, p := range found[1:] {
		d := strings.Count(p, string(os.PathSeparator))
		if d < bestDepth {
			best, bestDepth = p, d
		}
	}
	return best, nil
}

func installPayload(srcDir, destDir, goos string) (binaryPath, backupPath string, fresh bool, err error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", "", false, err
	}
	srcBin, err := findProxyBinary(srcDir, goos)
	if err != nil {
		return "", "", false, err
	}
	payloadRoot := filepath.Dir(srcBin)
	destBin := filepath.Join(destDir, proxyBinaryName(goos))
	_, destErr := os.Stat(destBin)
	fresh = os.IsNotExist(destErr)

	if destErr == nil {
		backupPath = destBin + ".old"
		if err := copyFile(destBin, backupPath); err != nil {
			return "", "", false, fmt.Errorf("backup current binary: %w", err)
		}
	}

	entries, err := os.ReadDir(payloadRoot)
	if err != nil {
		return "", "", false, err
	}
	for _, e := range entries {
		name := e.Name()
		src := filepath.Join(payloadRoot, name)
		dst := filepath.Join(destDir, name)
		if e.IsDir() {
			continue
		}
		switch {
		case strings.EqualFold(name, proxyBinaryName(goos)):
			if err := copyFile(src, dst); err != nil {
				return "", backupPath, fresh, fmt.Errorf("install binary: %w", err)
			}
			if goos != "windows" {
				_ = os.Chmod(dst, 0o755)
			}
		case strings.EqualFold(name, "dnscrypt-proxy.toml"):
			// never overwrite a live config
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				if err := copyFile(src, dst); err != nil {
					return "", backupPath, fresh, err
				}
			}
		case strings.EqualFold(name, "example-dnscrypt-proxy.toml"):
			if err := copyFile(src, dst); err != nil {
				return "", backupPath, fresh, err
			}
			toml := filepath.Join(destDir, "dnscrypt-proxy.toml")
			if _, err := os.Stat(toml); os.IsNotExist(err) {
				if err := copyFile(src, toml); err != nil {
					return "", backupPath, fresh, err
				}
			}
		default:
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				if err := copyFile(src, dst); err != nil {
					return "", backupPath, fresh, err
				}
			}
		}
	}

	if _, err := os.Stat(destBin); err != nil {
		return "", backupPath, fresh, fmt.Errorf("installed binary missing: %w", err)
	}
	return destBin, backupPath, fresh, nil
}

func restoreBackup(backupPath, destBin string) error {
	if backupPath == "" {
		return nil
	}
	if _, err := os.Stat(backupPath); err != nil {
		return err
	}
	return copyFile(backupPath, destBin)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if st, err := os.Stat(src); err == nil {
		_ = os.Chmod(tmp, st.Mode())
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
