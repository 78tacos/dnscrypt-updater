package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

var (
	errNoExecutable = errors.New("missing executable path")
	errNoRegistry   = errors.New("windows autostart registry is not available")
)

func writeFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func removeFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
