// Package detect locates a local dnscrypt-proxy binary and reads its CLI version.
//
// The authoritative version is `dnscrypt-proxy -version` / `--version` stdout.
// Windows PE FileVersion is intentionally unused (often empty; DNSCrypt/dnscrypt-proxy#2381).
package detect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/78tacos/dnscrypt-updater/internal/version"
)

const (
	SourceOverride = "override"
	SourceBinary   = "binary"
	SourceNone     = "not_found"

	execTimeout = 5 * time.Second
)

// Result is the outcome of looking up the installed dnscrypt-proxy version.
type Result struct {
	Found   bool
	Path    string
	Version version.Version
	Source  string
	Err     error
}

// Runner is the OS-facing side of detection (overridable in tests).
type Runner struct {
	GOOS             string
	LookPath         func(file string) (string, error)
	IsExecutable     func(path string) bool
	RunVersion       func(ctx context.Context, path string) (string, error)
	ServiceImagePath func() (string, bool)
	Getenv           func(string) string
	UserHomeDir      func() (string, error)
}

// DefaultRunner talks to the real OS.
func DefaultRunner() Runner {
	r := Runner{
		GOOS:         runtime.GOOS,
		LookPath:     exec.LookPath,
		IsExecutable: fileExists,
		Getenv:       os.Getenv,
		UserHomeDir:  os.UserHomeDir,
		RunVersion:   runVersionCLI,
	}
	r.ServiceImagePath = r.serviceImagePath
	return r
}

// Detect resolves the local version.
//
// override, if non-empty, is the user's manual current-version and always
// wins for comparison. Binary discovery still runs so the UI can show the path.
func (r Runner) Detect(ctx context.Context, configuredPath, override string) Result {
	out := Result{Source: SourceNone}

	path, pathErr := r.findBinary(configuredPath)
	if pathErr == nil && path != "" {
		out.Path = path
		ver, err := r.readCLIVersion(ctx, path)
		if err != nil {
			out.Err = err
		} else {
			out.Found = true
			out.Version = ver
			out.Source = SourceBinary
		}
	} else if pathErr != nil {
		out.Err = pathErr
	}

	override = strings.TrimSpace(override)
	if override == "" {
		if !out.Found && out.Err == nil {
			out.Err = errors.New("dnscrypt-proxy not found")
		}
		return out
	}

	v, err := version.Parse(override)
	if err != nil {
		out.Err = fmt.Errorf("invalid current_version override %q: %w", override, err)
		out.Found = false
		out.Source = SourceNone
		return out
	}
	out.Found = true
	out.Version = v
	out.Source = SourceOverride
	out.Err = nil
	return out
}

func (r Runner) findBinary(configuredPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" {
		if r.IsExecutable != nil && r.IsExecutable(configuredPath) {
			return configuredPath, nil
		}
		return "", fmt.Errorf("configured binary_path not found or not executable: %s", configuredPath)
	}

	name := r.binaryName()
	if r.LookPath != nil {
		if p, err := r.LookPath(name); err == nil && p != "" {
			if r.IsExecutable == nil || r.IsExecutable(p) {
				return p, nil
			}
		}
	}

	for _, p := range r.commonPaths() {
		if r.IsExecutable != nil && r.IsExecutable(p) {
			return p, nil
		}
	}

	if r.ServiceImagePath != nil {
		if p, ok := r.ServiceImagePath(); ok && p != "" {
			if r.IsExecutable == nil || r.IsExecutable(p) {
				return p, nil
			}
		}
	}
	return "", errors.New("dnscrypt-proxy not found on PATH or common install locations")
}

func (r Runner) binaryName() string {
	if r.GOOS == "windows" {
		return "dnscrypt-proxy.exe"
	}
	return "dnscrypt-proxy"
}

func (r Runner) commonPaths() []string {
	var paths []string
	if r.GOOS == "windows" {
		paths = append(paths, r.windowsCommonPaths()...)
	} else {
		paths = append(paths, r.unixCommonPaths()...)
	}
	return paths
}

func (r Runner) getenv(k string) string {
	if r.Getenv != nil {
		return r.Getenv(k)
	}
	return ""
}

func (r Runner) windowsCommonPaths() []string {
	name := "dnscrypt-proxy.exe"
	var dirs []string
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA", "ProgramData"} {
		if v := r.getenv(key); v != "" {
			dirs = append(dirs, filepath.Join(v, "dnscrypt-proxy"))
		}
	}
	if home, err := r.home(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, "scoop", "apps", "dnscrypt-proxy", "current"),
			filepath.Join(home, "dnscrypt-proxy"),
		)
	}
	if choco := r.getenv("ChocolateyInstall"); choco != "" {
		dirs = append(dirs, filepath.Join(choco, "bin"))
	}
	dirs = append(dirs, `C:\dnscrypt-proxy`)

	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, filepath.Join(d, name))
	}
	return out
}

func (r Runner) unixCommonPaths() []string {
	name := "dnscrypt-proxy"
	paths := []string{
		"/opt/dnscrypt-proxy/" + name,
		"/usr/local/sbin/" + name,
		"/usr/local/bin/" + name,
		"/usr/sbin/" + name,
		"/usr/bin/" + name,
	}
	if r.GOOS == "darwin" {
		paths = append([]string{
			"/opt/homebrew/bin/" + name,
			"/usr/local/opt/dnscrypt-proxy/sbin/" + name,
		}, paths...)
	}
	if home, err := r.home(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, "dnscrypt-proxy", name))
	}
	return paths
}

func (r Runner) home() (string, error) {
	if r.UserHomeDir != nil {
		return r.UserHomeDir()
	}
	return os.UserHomeDir()
}

func (r Runner) readCLIVersion(ctx context.Context, path string) (version.Version, error) {
	if r.RunVersion == nil {
		return version.Version{}, errors.New("no version runner configured")
	}
	out, err := r.RunVersion(ctx, path)
	if err != nil {
		return version.Version{}, err
	}
	return ParseCLIVersion(out)
}

func (r Runner) serviceImagePath() (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()
	switch r.GOOS {
	case "windows":
		cmd := exec.CommandContext(ctx, "sc", "qc", "dnscrypt-proxy")
		b, err := cmd.CombinedOutput()
		if err != nil {
			return "", false
		}
		return ParseSCBinaryPath(string(b))
	case "linux":
		cmd := exec.CommandContext(ctx, "systemctl", "show", "-p", "ExecStart", "dnscrypt-proxy")
		b, err := cmd.CombinedOutput()
		if err != nil {
			return "", false
		}
		return ParseSystemctlExecStart(string(b))
	default:
		return "", false
	}
}

func runVersionCLI(ctx context.Context, path string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, execTimeout)
		defer cancel()
	}
	out, err := runOne(ctx, path, "-version")
	if err == nil && strings.TrimSpace(out) != "" {
		return out, nil
	}
	out2, err2 := runOne(ctx, path, "--version")
	if err2 == nil && strings.TrimSpace(out2) != "" {
		return out2, nil
	}
	if err != nil {
		return "", fmt.Errorf("run %s -version: %w", path, err)
	}
	return "", fmt.Errorf("run %s --version: %w", path, err2)
}

func runOne(ctx context.Context, path string, arg string) (string, error) {
	cmd := exec.CommandContext(ctx, path, arg)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	// Do not inspect Windows PE FileVersion. Existence + CLI is enough.
	return true
}
