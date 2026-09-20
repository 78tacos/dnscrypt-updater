package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/78tacos/dnscrypt-updater/internal/apply"
	"github.com/78tacos/dnscrypt-updater/internal/notify"
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
		Check:        rt.checkProxyConfig,
		StopService:  ap.StopService,
		StartService: ap.StartService,
		PendingDir:   func() string { return rt.Paths.Pending },
		OnPending: func(st proxyconf.PendingStatus) {
			rt.pingMenu()
			if st.Present {
				if nerr := notify.SettingsQueued(st.Dir); nerr != nil {
					rt.Log.Warn("pending settings notification", "err", nerr)
				}
			}
		},
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

func (rt *Runtime) checkProxyConfig(ctx context.Context, bin, configPath string) error {
	if rt.Applier != nil && rt.Applier.RunCheck != nil {
		return rt.Applier.RunCheck(ctx, bin, configPath)
	}
	return apply.CheckConfig(ctx, bin, configPath)
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
	res, err := proxyconf.Commit(ctx, proxyconf.ApplyEnv{
		InstallDir:    dir,
		BinaryPath:    bin,
		ManageService: manage,
		Check:         rt.checkProxyConfig,
		StopService:   ap.StopService,
		StartService:  ap.StartService,
	}, staging)
	if err != nil {
		return res, err
	}
	if pending := strings.TrimSpace(rt.Paths.Pending); pending != "" {
		_ = proxyconf.ClearPending(pending)
	}
	rt.pingMenu()
	return res, nil
}

// ApplyPending copies a queued AppData bundle into the install dir.
// On Windows this prompts UAC for a one-shot -apply-config; the tray stays in userspace.
func (rt *Runtime) ApplyPending(ctx context.Context) (proxyconf.ApplyResult, error) {
	pending := strings.TrimSpace(rt.Paths.Pending)
	st := proxyconf.ReadPending(pending)
	if !st.Present {
		return proxyconf.ApplyResult{}, fmt.Errorf("no pending settings in %s", pending)
	}
	rt.Log.Info("applying pending settings", "dir", pending, "install", st.InstallDir)
	if rt.applier().NeedsWindowsElevation() {
		if err := rt.elevateApply(pending); err != nil {
			_ = notify.SettingsApplyFailed(err)
			return proxyconf.ApplyResult{}, err
		}
		_ = proxyconf.ClearPending(pending)
		rt.pingMenu()
		msg := "Applied pending settings with administrator permission."
		_ = notify.SettingsApplied(msg)
		dir, _ := rt.proxyPaths()
		return proxyconf.ApplyResult{
			TomlPath: filepath.Join(dir, "dnscrypt-proxy.toml"),
			Message:  msg,
			Applied:  true,
		}, nil
	}
	res, err := rt.ApplyStaged(ctx, pending)
	if err != nil {
		_ = notify.SettingsApplyFailed(err)
		return res, err
	}
	_ = notify.SettingsApplied(res.Message)
	return res, nil
}

func (rt *Runtime) pingMenu() {
	if rt == nil || rt.menuPing == nil {
		return
	}
	select {
	case rt.menuPing <- struct{}{}:
	default:
	}
}
