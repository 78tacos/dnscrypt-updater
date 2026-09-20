package proxyconf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
}

// ApplyResult is the outcome of a commit.
type ApplyResult struct {
	TomlPath string `json:"toml_path"`
	Message  string `json:"message"`
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
		_ = env.StopService(ctx, env.BinaryPath)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(staging, e.Name())
		dst := filepath.Join(env.InstallDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			if err := copyFile(dst, backupPath(dst)); err != nil {
				rollback()
				return out, fmt.Errorf("backup %s: %w", dst, err)
			}
		}
		if err := copyFile(src, dst); err != nil {
			rollback()
			return out, fmt.Errorf("write %s: %w", dst, err)
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
	out.Message = "Saved dnscrypt-proxy.toml and restarted the service."
	if !env.ManageService {
		out.Message = "Saved dnscrypt-proxy.toml. Restart dnscrypt-proxy to apply."
	}
	return out, nil
}
