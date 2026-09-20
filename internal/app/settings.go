package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/78tacos/dnscrypt-updater/internal/apply"
	"github.com/78tacos/dnscrypt-updater/internal/openurl"
	"github.com/78tacos/dnscrypt-updater/internal/proxyconf"
	"github.com/78tacos/dnscrypt-updater/internal/settingsui"
)

func (rt *Runtime) proxyPaths() (installDir, binaryPath string) {
	rt.mu.Lock()
	cfg := rt.cfg
	last := rt.last
	rt.mu.Unlock()

	bin := firstNonEmpty(rt.Opts.BinaryPath, cfg.BinaryPath, last.BinaryPath)
	dir := strings.TrimSpace(firstNonEmpty(rt.Opts.InstallDir, cfg.InstallDir))
	if dir == "" && bin != "" && !strings.Contains(strings.ToLower(bin), "scoop") && !strings.Contains(strings.ToLower(bin), "chocolatey") {
		dir = filepath.Dir(bin)
	}
	if dir == "" {
		dir = apply.DefaultInstallDir(runtime.GOOS, os.Getenv)
	}
	if bin == "" {
		bin = filepath.Join(dir, apply.ProxyBinaryName(runtime.GOOS))
	}
	return dir, bin
}

func (rt *Runtime) settingsOpts() settingsui.Options {
	ap := rt.applier()
	return settingsui.Options{
		Locate: func() (string, string, bool) {
			dir, bin := rt.proxyPaths()
			rt.mu.Lock()
			manage := rt.cfg.ManageService && !rt.Opts.NoService
			rt.mu.Unlock()
			return dir, bin, manage
		},
		Log: rt.Log,
		NeedsElevation: func() bool {
			return ap.NeedsWindowsElevation()
		},
		Elevate: func(ctx context.Context, staging string) error {
			return rt.elevateApply(staging)
		},
		Check:        apply.CheckConfig,
		StopService:  ap.StopService,
		StartService: ap.StartService,
	}
}

func (rt *Runtime) OpenSettings(ctx context.Context) (string, error) {
	rt.settingsMu.Lock()
	defer rt.settingsMu.Unlock()
	if rt.settingsURL == "" {
		srv, err := settingsui.New(rt.settingsOpts())
		if err != nil {
			return "", err
		}
		u, err := srv.Start(ctx)
		if err != nil {
			return "", err
		}
		rt.settingsURL = u
		rt.Log.Info("settings ui listening", "url", u)
	}
	if err := openurl.OpenLoopback(rt.settingsURL); err != nil {
		rt.Log.Warn("open settings url", "err", err)
	}
	return rt.settingsURL, nil
}

// RunSettingsUI starts the loopback UI and blocks until ctx is cancelled.
func (rt *Runtime) RunSettingsUI(ctx context.Context) error {
	srv, err := settingsui.New(rt.settingsOpts())
	if err != nil {
		return err
	}
	u, err := srv.Start(ctx)
	if err != nil {
		return err
	}
	rt.Log.Info("settings ui", "url", u)
	if !rt.Opts.Quiet {
		fmt.Fprintln(os.Stdout, u)
	}
	_ = openurl.OpenLoopback(u)
	<-ctx.Done()
	return nil
}

func (rt *Runtime) elevateApply(staging string) error {
	dir, _ := rt.proxyPaths()
	args := []string{"-apply-config", "-quiet", "-config", rt.Paths.File, "-staging", staging}
	if dir != "" {
		args = append(args, "-install-dir", dir)
	}
	rt.Log.Info("requesting administrator permission to save dnscrypt-proxy settings")
	code, err := rt.applier().RelaunchElevated(args)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("elevated apply-config exited %d", code)
	}
	return nil
}

// ApplyStaged commits a staging directory written by the settings UI (elevated path).
func (rt *Runtime) ApplyStaged(ctx context.Context, staging string) (proxyconf.ApplyResult, error) {
	if strings.TrimSpace(staging) == "" {
		return proxyconf.ApplyResult{}, fmt.Errorf("-staging is required with -apply-config")
	}
	dir, bin := rt.proxyPaths()
	rt.mu.Lock()
	manage := rt.cfg.ManageService && !rt.Opts.NoService
	rt.mu.Unlock()
	ap := rt.applier()
	return proxyconf.Commit(ctx, proxyconf.ApplyEnv{
		InstallDir:    dir,
		BinaryPath:    bin,
		ManageService: manage,
		Check:         apply.CheckConfig,
		StopService:   ap.StopService,
		StartService:  ap.StartService,
	}, staging)
}
