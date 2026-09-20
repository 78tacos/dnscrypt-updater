package proxyconf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const tomlName = "dnscrypt-proxy.toml"

// ApplyRequest is a UI save.
type ApplyRequest struct {
	Patches []Patch      `json:"patches"`
	Files   []FileChange `json:"files"`
	Preset  string       `json:"preset,omitempty"`
}

// ApplyEnv is the install location and hooks for check/service.
type ApplyEnv struct {
	InstallDir    string
	BinaryPath    string
	ManageService bool
	Check         func(ctx context.Context, bin, configPath string) error
	StopService   func(ctx context.Context, bin string) error
	StartService  func(ctx context.Context, bin string) error
	// WriteErr, if set, is returned after -check / service stop instead of
	// writing files. Tests use this to simulate access-denied without relying
	// on OS-specific read-only directories (Windows chmod is a no-op for owners).
	WriteErr error
}

// ApplyResult is the outcome of a commit or a queued pending bundle.
type ApplyResult struct {
	TomlPath   string   `json:"toml_path"`
	Message    string   `json:"message"`
	Applied    bool     `json:"applied"`
	Pending    bool     `json:"pending"`
	PendingDir string   `json:"pending_dir,omitempty"`
	Files      []string `json:"files,omitempty"`
}

func tomlPath(installDir string) string {
	return filepath.Join(installDir, tomlName)
}

// Stage writes patched toml + list files into destDir (usually a temp dir).
func Stage(installDir, destDir string, req ApplyRequest, cat Catalog) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	srcToml := tomlPath(installDir)
	raw, err := os.ReadFile(srcToml)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcToml, err)
	}
	patches := req.Patches
	if id := strings.TrimSpace(req.Preset); id != "" {
		found := false
		for _, p := range Presets() {
			if p.ID == id {
				patches = append(append([]Patch{}, p.Patches...), patches...)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown preset %q", id)
		}
	}
	next, err := ApplyPatches(string(raw), patches, cat)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(destDir, tomlName), next); err != nil {
		return err
	}

	// Copy existing companion files so relative paths in -check resolve.
	for _, c := range companionFiles {
		src := filepath.Join(installDir, c.Live)
		dst := filepath.Join(destDir, c.Live)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, dst); err != nil {
				return err
			}
		}
	}
	for _, fc := range req.Files {
		dst, err := resolveListPath(destDir, fc.Name)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(dst, fc.Content); err != nil {
			return err
		}
	}
	// If a patch enables a list file that doesn't exist yet, seed from example.
	staged := parseDoc(next)
	vals := staged.values(cat)
	for _, c := range companionFiles {
		v, ok := vals[c.Path]
		if !ok || !v.Present {
			continue
		}
		live := filepath.Join(destDir, c.Live)
		if _, err := os.Stat(live); err == nil {
			continue
		}
		ex, err := ExampleListFile(c.Example)
		if err != nil {
			continue
		}
		if err := writeFileAtomic(live, string(ex)); err != nil {
			return err
		}
	}
	return nil
}

// Commit copies a staging directory over the live install after -check.
func Commit(ctx context.Context, env ApplyEnv, staging string) (ApplyResult, error) {
	out := ApplyResult{TomlPath: tomlPath(env.InstallDir)}
	if env.InstallDir == "" {
		return out, fmt.Errorf("install dir is empty")
	}
	stagedToml := filepath.Join(staging, tomlName)
	if _, err := os.Stat(stagedToml); err != nil {
		return out, fmt.Errorf("staging missing %s: %w", tomlName, err)
	}
	if env.Check != nil && env.BinaryPath != "" {
		if err := env.Check(ctx, env.BinaryPath, stagedToml); err != nil {
			return out, err
		}
	}

	var copied []string
	rollback := func() {
		for i := len(copied) - 1; i >= 0; i-- {
			p := copied[i]
			bak := backupPath(p)
			if _, err := os.Stat(bak); err == nil {
				_ = copyFile(bak, p)
			}
		}
	}

	entries, err := os.ReadDir(staging)
	if err != nil {
		return out, err
	}
	if env.ManageService && env.StopService != nil && env.BinaryPath != "" {
		if err := env.StopService(ctx, env.BinaryPath); err != nil {
			// Keep going: some installs stop cleanly even when this reports an error,
			// and a later write error still triggers elevation / pending fallback.
			// A hard privilege failure here is handled when the write fails.
			_ = err
		}
	}
	if env.WriteErr != nil {
		return out, env.WriteErr
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !commitName(e.Name()) {
			continue
		}
		src := filepath.Join(staging, e.Name())
		dst := filepath.Join(env.InstallDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			if err := copyFile(dst, backupPath(dst)); err != nil {
				if retryErr := retryAfterStop(ctx, env, err, func() error {
					return copyFile(dst, backupPath(dst))
				}); retryErr != nil {
					rollback()
					return out, fmt.Errorf("backup %s: %w", dst, retryErr)
				}
			}
		}
		if err := copyFile(src, dst); err != nil {
			if retryErr := retryAfterStop(ctx, env, err, func() error {
				return copyFile(src, dst)
			}); retryErr != nil {
				rollback()
				return out, fmt.Errorf("write %s: %w", dst, retryErr)
			}
		}
		copied = append(copied, dst)
	}
	if env.Check != nil && env.BinaryPath != "" {
		if err := env.Check(ctx, env.BinaryPath, out.TomlPath); err != nil {
			rollback()
			if env.ManageService && env.StartService != nil {
				_ = env.StartService(ctx, env.BinaryPath)
			}
			return out, err
		}
	}
	if env.ManageService && env.StartService != nil && env.BinaryPath != "" {
		if err := env.StartService(ctx, env.BinaryPath); err != nil {
			rollback()
			_ = env.StartService(ctx, env.BinaryPath)
			return out, fmt.Errorf("service start failed: %w", err)
		}
	}
	out.Applied = true
	out.Message = "Saved dnscrypt-proxy.toml and restarted the service."
	if !env.ManageService {
		out.Message = "Saved dnscrypt-proxy.toml. Restart dnscrypt-proxy to apply."
	}
	return out, nil
}

// TryCommit writes the live install if possible.
//
// Order matters: always attempt an in-place Commit first (which stops the
// service before writing). A running dnscrypt-proxy often makes a naive
// writable probe fail even when the directory ACL does not need admin; jumping
// straight to UAC/pending in that case was wrong. Only elevate or queue pending
// after Commit fails with a detectable privilege / sharing error.
func TryCommit(ctx context.Context, env ApplyEnv, staging, pendingDir string, writable bool, elevate func(context.Context, string) error) (ApplyResult, error) {
	_ = writable // retained for callers / UI; no longer gates elevation.
	// Commit runs -check once before writing; do not pre-check here (avoids an
	// extra dnscrypt-proxy console flash on Windows).
	res, err := Commit(ctx, env, staging)
	if err == nil {
		_ = ClearPending(pendingDir)
		return res, nil
	}
	if !IsPrivilegeError(err) {
		return res, err
	}
	if elevate != nil {
		if eerr := elevate(ctx, staging); eerr != nil {
			// Settings already passed -check. If admin apply did not land them
			// (UAC cancel, opaque "exit status 1", service failure, …), park them
			// in AppData instead of returning a useless bare exec error.
			if q, qerr := queuePending(staging, pendingDir, env.InstallDir, eerr); qerr == nil {
				return q, nil
			}
			return ApplyResult{}, eerr
		}
		_ = ClearPending(pendingDir)
		return ApplyResult{
			TomlPath: tomlPath(env.InstallDir),
			Message:  "Saved dnscrypt-proxy settings with administrator permission.",
			Applied:  true,
		}, nil
	}
	return queuePending(staging, pendingDir, env.InstallDir, err)
}

// retryAfterStop tries StopService once when a write fails because the live
// toml is locked by the running proxy (common when manage_service is off).
func retryAfterStop(ctx context.Context, env ApplyEnv, first error, fn func() error) error {
	if first == nil {
		return nil
	}
	if !IsPrivilegeError(first) || env.StopService == nil || strings.TrimSpace(env.BinaryPath) == "" {
		return first
	}
	_ = env.StopService(ctx, env.BinaryPath)
	if err := fn(); err != nil {
		return first
	}
	return nil
}

func queuePending(staging, pendingDir, installDir string, cause error) (ApplyResult, error) {
	if strings.TrimSpace(pendingDir) == "" {
		return ApplyResult{}, cause
	}
	st, err := SavePending(staging, pendingDir, installDir, cause.Error())
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w (also failed to queue pending settings: %v)", cause, err)
	}
	return ApplyResult{
		TomlPath:   tomlPath(installDir),
		Message:    pendingOfferMessage(installDir, st.Dir),
		Pending:    true,
		PendingDir: st.Dir,
		Files:      st.Files,
	}, nil
}

func pendingOfferMessage(installDir, pendingDir string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Could not write %s (access denied or the file is in use by dnscrypt-proxy). Settings are queued in %s. Right-click the tray icon and choose Apply pending settings (may prompt for Administrator), or download the zip from this page.", installDir, pendingDir)
	}
	return fmt.Sprintf("Could not write %s (permission denied or file in use). Settings are queued in %s. Apply from the tray, or run: sudo dnscrypt-proxy-updater -apply-pending — or download the zip from this page.", installDir, pendingDir)
}

func commitName(name string) bool {
	if name == tomlName {
		return true
	}
	return allowedCompanion(name)
}
