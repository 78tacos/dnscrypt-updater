package proxyconf

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const manifestName = "manifest.json"

// Manifest describes a pending settings bundle in the user config dir.
type Manifest struct {
	InstallDir string    `json:"install_dir"`
	SavedAt    time.Time `json:"saved_at"`
	Files      []string  `json:"files"`
	Reason     string    `json:"reason,omitempty"`
}

// PendingStatus is shown in the settings UI and tray.
type PendingStatus struct {
	Present    bool     `json:"present"`
	Dir        string   `json:"dir,omitempty"`
	InstallDir string   `json:"install_dir,omitempty"`
	Files      []string `json:"files,omitempty"`
	SavedAt    string   `json:"saved_at,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// IsPrivilegeError reports permission / UAC / elevation failures we can fall back from.
func IsPrivilegeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	var pe *os.PathError
	if errors.As(err, &pe) && (errors.Is(pe.Err, os.ErrPermission) || os.IsPermission(pe)) {
		return true
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"permission denied",
		"access is denied",
		"operation not permitted",
		"administrator permission was declined",
		"administrator apply failed",
		"elevated process failed",
		"automatic elevation is only supported",
		"the requested operation requires elevation",
		"being used by another process",
		"sharing violation",
		"cannot access the file because it is being used",
	}
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

// SavePending copies checked toml/list files into pendingDir (replacing any previous bundle).
func SavePending(staging, pendingDir, installDir, reason string) (PendingStatus, error) {
	if strings.TrimSpace(pendingDir) == "" {
		return PendingStatus{}, fmt.Errorf("pending dir is empty")
	}
	if err := ClearPending(pendingDir); err != nil {
		return PendingStatus{}, err
	}
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		return PendingStatus{}, err
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return PendingStatus{}, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !commitName(e.Name()) {
			continue
		}
		if err := copyFile(filepath.Join(staging, e.Name()), filepath.Join(pendingDir, e.Name())); err != nil {
			return PendingStatus{}, err
		}
		files = append(files, e.Name())
	}
	if len(files) == 0 {
		return PendingStatus{}, fmt.Errorf("no settings files to queue")
	}
	man := Manifest{
		InstallDir: installDir,
		SavedAt:    time.Now().UTC(),
		Files:      files,
		Reason:     reason,
	}
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return PendingStatus{}, err
	}
	if err := os.WriteFile(filepath.Join(pendingDir, manifestName), append(b, '\n'), 0o644); err != nil {
		return PendingStatus{}, err
	}
	return ReadPending(pendingDir), nil
}

// ReadPending returns the current queued bundle, if any.
func ReadPending(pendingDir string) PendingStatus {
	out := PendingStatus{Dir: pendingDir}
	if strings.TrimSpace(pendingDir) == "" {
		return out
	}
	toml := filepath.Join(pendingDir, tomlName)
	if _, err := os.Stat(toml); err != nil {
		return out
	}
	out.Present = true
	if b, err := os.ReadFile(filepath.Join(pendingDir, manifestName)); err == nil {
		var man Manifest
		if json.Unmarshal(b, &man) == nil {
			out.InstallDir = man.InstallDir
			out.Files = man.Files
			out.Reason = man.Reason
			if !man.SavedAt.IsZero() {
				out.SavedAt = man.SavedAt.Format(time.RFC3339)
			}
		}
	}
	if len(out.Files) == 0 {
		entries, _ := os.ReadDir(pendingDir)
		for _, e := range entries {
			if !e.IsDir() && commitName(e.Name()) {
				out.Files = append(out.Files, e.Name())
			}
		}
	}
	return out
}

// ClearPending removes a queued bundle.
func ClearPending(pendingDir string) error {
	if strings.TrimSpace(pendingDir) == "" {
		return nil
	}
	if _, err := os.Stat(pendingDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(pendingDir)
}

// WritePendingZip writes a zip of queued toml/list files (not the manifest).
func WritePendingZip(pendingDir string, w io.Writer) error {
	st := ReadPending(pendingDir)
	if !st.Present {
		return fmt.Errorf("no pending settings")
	}
	zw := zip.NewWriter(w)
	for _, name := range st.Files {
		if !commitName(name) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(pendingDir, name))
		if err != nil {
			_ = zw.Close()
			return err
		}
		fw, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := fw.Write(b); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}
