package apply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
)

// Options control a single install/update of official dnscrypt-proxy.
type Options struct {
	InstallDir     string
	ExistingBinary string
	SetSystemDNS   bool
	ManageService  bool
	SkipElevation  bool
	ElevatedArgs   []string
}

// Result is the outcome of Apply.
type Result struct {
	Version          string `json:"version"`
	InstallDir       string `json:"install_dir"`
	BinaryPath       string `json:"binary_path"`
	BackupPath       string `json:"backup_path,omitempty"`
	FreshInstall     bool   `json:"fresh_install"`
	Verified         bool   `json:"verified"`
	ServiceInstalled bool   `json:"service_installed"`
	ServiceStarted   bool   `json:"service_started"`
	DNSUpdated       bool   `json:"dns_updated"`
	Elevated         bool   `json:"elevated,omitempty"`
	Message          string `json:"message"`
}

// Applier downloads the official signed archive and installs it.
type Applier struct {
	HTTP      *http.Client
	Log       *slog.Logger
	GOOS      string
	GOARCH    string
	UserAgent string
	Getenv    func(string) string
	Service   ServiceManager
	DNS       DNSSetter
	Elevated  func() bool
	Relaunch  func(args []string) (int, error)
	RunCheck  func(ctx context.Context, bin, configPath string) error
	MkdirTemp func(dir, pattern string) (string, error)
	// AllowDownload, if set, replaces ValidateAssetURL (tests).
	AllowDownload func(rawURL string) error
	// MinisignPubKey, if set, replaces the official hardcoded key (tests).
	MinisignPubKey string
}

func (a *Applier) log() *slog.Logger {
	if a != nil && a.Log != nil {
		return a.Log
	}
	return slog.Default()
}

func (a *Applier) service() ServiceManager {
	if a != nil && a.Service != nil {
		return a.Service
	}
	return defaultService(a.goos())
}

func (a *Applier) dns() DNSSetter {
	if a != nil && a.DNS != nil {
		return a.DNS
	}
	return nativeDNS{}
}

func (a *Applier) elevated() bool {
	if a != nil && a.Elevated != nil {
		return a.Elevated()
	}
	return isElevated()
}

func (a *Applier) relaunch(args []string) (int, error) {
	if a != nil && a.Relaunch != nil {
		return a.Relaunch(args)
	}
	return relaunchElevatedAndWait(args)
}

func (a *Applier) runCheck(ctx context.Context, bin, configPath string) error {
	if a != nil && a.RunCheck != nil {
		return a.RunCheck(ctx, bin, configPath)
	}
	return runConfigCheck(ctx, bin, configPath)
}

func (a *Applier) mkdirTemp(dir, pattern string) (string, error) {
	if a != nil && a.MkdirTemp != nil {
		return a.MkdirTemp(dir, pattern)
	}
	return os.MkdirTemp(dir, pattern)
}

func (a *Applier) pubKey() string {
	if a != nil && a.MinisignPubKey != "" {
		return a.MinisignPubKey
	}
	return githubrel.MinisignPubKey
}

// NeedsWindowsElevation reports whether this process should relaunch via UAC.
func (a *Applier) NeedsWindowsElevation() bool {
	return a.goos() == "windows" && !a.elevated()
}

// RelaunchElevated reruns this executable with admin rights and waits.
func (a *Applier) RelaunchElevated(args []string) (int, error) {
	return a.relaunch(args)
}

// Apply downloads, minisign-verifies, and installs the official archive for this OS.
func (a *Applier) Apply(ctx context.Context, archive githubrel.SignedArchive, tag string, opts Options) (Result, error) {
	out := Result{}
	tag = strings.TrimSpace(tag)
	out.Version = strings.TrimPrefix(strings.TrimPrefix(tag, "v"), "V")

	if archive.Archive.BrowserDownloadURL == "" || archive.Minisig.BrowserDownloadURL == "" {
		return out, errors.New("official signed archive URL missing")
	}

	work, err := a.mkdirTemp("", "dnscrypt-proxy-updater-*")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(work)

	archivePath := filepath.Join(work, filepath.Base(archive.Archive.Name))
	sigPath := filepath.Join(work, filepath.Base(archive.Minisig.Name))
	a.log().Info("downloading official archive", "asset", archive.Archive.Name, "url", archive.Archive.BrowserDownloadURL)
	if err := a.download(ctx, archive.Archive.BrowserDownloadURL, archivePath); err != nil {
		return out, err
	}
	if err := a.download(ctx, archive.Minisig.BrowserDownloadURL, sigPath); err != nil {
		return out, err
	}
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return out, err
	}
	if err := verifyArchive(archivePath, sig, a.pubKey()); err != nil {
		return out, err
	}
	out.Verified = true

	extractDir := filepath.Join(work, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return out, err
	}
	if err := extractArchive(archivePath, extractDir); err != nil {
		return out, err
	}

	srcBin, err := findProxyBinary(extractDir, a.goos())
	if err != nil {
		return out, err
	}

	destDir := resolveInstallDir(opts.InstallDir, opts.ExistingBinary, a.goos(), a.getenv)
	out.InstallDir = destDir
	tomlPath := filepath.Join(destDir, "dnscrypt-proxy.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		if err := a.runCheck(ctx, srcBin, tomlPath); err != nil {
			return out, fmt.Errorf("new binary rejected existing config: %w", err)
		}
	}

	svc := a.service()
	stopBin := opts.ExistingBinary
	if stopBin == "" {
		candidate := filepath.Join(destDir, proxyBinaryName(a.goos()))
		if _, err := os.Stat(candidate); err == nil {
			stopBin = candidate
		}
	}
	if opts.ManageService && stopBin != "" {
		st, _ := svc.Query(ctx)
		if st.Running {
			if err := svc.Stop(ctx, stopBin); err != nil {
				a.log().Warn("stop service", "err", err)
			}
		}
	}

	binPath, backup, fresh, err := installPayload(extractDir, destDir, a.goos())
	if err != nil {
		return out, err
	}
	out.BinaryPath = binPath
	out.BackupPath = backup
	out.FreshInstall = fresh

	tomlPath = filepath.Join(destDir, "dnscrypt-proxy.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		if err := a.runCheck(ctx, binPath, tomlPath); err != nil {
			if backup != "" {
				_ = restoreBackup(backup, binPath)
			}
			return out, fmt.Errorf("installed binary failed -check: %w", err)
		}
	}

	if opts.ManageService {
		st, _ := svc.Query(ctx)
		if !st.Installed {
			if err := svc.Install(ctx, binPath); err != nil {
				return out, fmt.Errorf("files installed to %s but service install failed: %w", destDir, err)
			}
		}
		out.ServiceInstalled = true
		if err := svc.Start(ctx, binPath); err != nil {
			if backup != "" {
				_ = restoreBackup(backup, binPath)
				_ = svc.Start(ctx, binPath)
			}
			return out, fmt.Errorf("service start failed: %w", err)
		}
		out.ServiceStarted = true
	}

	if opts.SetSystemDNS {
		if err := a.dns().SetLoopback(ctx); err != nil {
			a.log().Warn("system DNS not changed", "err", err)
			out.Message = fmt.Sprintf("Installed dnscrypt-proxy %s to %s (minisign OK). Could not set system DNS: %v", out.Version, destDir, err)
			return out, nil
		}
		out.DNSUpdated = true
	}

	out.Message = summarize(out)
	return out, nil
}

func summarize(r Result) string {
	var b strings.Builder
	if r.FreshInstall {
		fmt.Fprintf(&b, "Installed dnscrypt-proxy %s to %s", r.Version, r.InstallDir)
	} else {
		fmt.Fprintf(&b, "Updated dnscrypt-proxy to %s in %s", r.Version, r.InstallDir)
	}
	if r.Verified {
		b.WriteString(" (minisign OK)")
	}
	if r.ServiceStarted {
		b.WriteString("; service running")
	} else if r.ServiceInstalled {
		b.WriteString("; service installed")
	}
	if r.DNSUpdated {
		b.WriteString("; system DNS set to 127.0.0.1")
	}
	b.WriteByte('.')
	return b.String()
}
