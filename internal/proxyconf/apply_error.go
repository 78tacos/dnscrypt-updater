package proxyconf

import (
	"os"
	"path/filepath"
	"strings"
)

const applyErrorName = ".apply-error.txt"

// WriteApplyError records why an elevated -apply-config failed so the
// unelevated parent can show a real message instead of bare "exit status 1".
func WriteApplyError(staging string, err error) {
	if strings.TrimSpace(staging) == "" || err == nil {
		return
	}
	_ = os.WriteFile(filepath.Join(staging, applyErrorName), []byte(err.Error()+"\n"), 0o644)
}

// ReadApplyError returns the elevated apply failure reason, if any.
func ReadApplyError(staging string) string {
	if strings.TrimSpace(staging) == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(staging, applyErrorName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ClearApplyError removes a stale elevated failure note.
func ClearApplyError(staging string) {
	if strings.TrimSpace(staging) == "" {
		return
	}
	_ = os.Remove(filepath.Join(staging, applyErrorName))
}
