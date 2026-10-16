package proxyconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const applyMetaName = ".apply-meta.json"

// ApplyMeta is written into a staging directory before UAC so the elevated
// child still knows the real install/binary paths even if command-line
// quoting mangles -install-dir / -binary-path.
type ApplyMeta struct {
	InstallDir string `json:"install_dir,omitempty"`
	BinaryPath string `json:"binary_path,omitempty"`
}

// WriteApplyMeta records install paths for an elevated -apply-config child.
func WriteApplyMeta(staging string, meta ApplyMeta) error {
	if strings.TrimSpace(staging) == "" {
		return nil
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(staging, applyMetaName), append(b, '\n'), 0o644)
}

// ReadApplyMeta returns staging apply paths, if present.
func ReadApplyMeta(staging string) ApplyMeta {
	var out ApplyMeta
	if strings.TrimSpace(staging) == "" {
		return out
	}
	b, err := os.ReadFile(filepath.Join(staging, applyMetaName))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, &out)
	return out
}
