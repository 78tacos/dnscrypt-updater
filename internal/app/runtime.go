package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/78tacos/dnscrypt-updater/internal/apply"
	"github.com/78tacos/dnscrypt-updater/internal/check"
	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/detect"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
	"github.com/78tacos/dnscrypt-updater/internal/notify"
	"github.com/78tacos/dnscrypt-updater/internal/openurl"
)

const AppName = "dnscrypt-proxy-updater"

// AppVersion is this companion's semver. Release builds override it with -ldflags.
var AppVersion = "2.0.0"

// ErrTrayUnavailable is returned by builds that were compiled without a system tray.
var ErrTrayUnavailable = errors.New("system tray is not available in this build")

// Options are CLI-derived settings.
type Options struct {
	ConfigPath     string
	CheckOnce      bool
	JSON           bool
	Quiet          bool
	CurrentVersion string
	BinaryPath     string
	ForceNotify    bool
	NoNotify       bool
	Install        bool
	NoDNS          bool
	NoService      bool
	InstallDir     string
	Configure      bool
	ApplyConfig    bool
	Staging        string
}

// Runtime is the long-lived updater process.
type Runtime struct {
	Opts   Options
	Paths  config.Paths
	Engine check.Engine
	Log    *slog.Logger

	mu     sync.Mutex
	cfg    config.File
	state  config.State
	last   check.Result
	cancel context.CancelFunc

	Applier *apply.Applier

	settingsMu  sync.Mutex
	settingsURL string
}

func NewRuntime(opts Options, log *slog.Logger) (*Runtime, error) {
	paths, err := config.ResolvePaths(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	if err := paths.EnsureDir(); err != nil {
		return nil, err
	}
	cfg, err := config.LoadFile(paths.File)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(paths.File); os.IsNotExist(err) {
		if err := config.SaveFile(paths.File, cfg); err != nil {
			return nil, err
		}
	}
	st, err := config.LoadState(paths.State)
	if err != nil {
		return nil, err
	}
	if v := opts.CurrentVersion; v != "" {
		cfg.CurrentVersion = v
	}
	if p := opts.BinaryPath; p != "" {
		cfg.BinaryPath = p
	}
	if opts.NoNotify {
		cfg.Notify = false
	}
	gh := &githubrel.Client{UserAgent: UserAgent()}
	eng := check.Engine{
		GitHub: gh,
		Detect: detect.DefaultRunner(),
	}
	if log == nil {
		log = slog.Default()
	}
	ap := &apply.Applier{Log: log, UserAgent: UserAgent(), Getenv: os.Getenv}
	return &Runtime{Opts: opts, Paths: paths, Engine: eng, Log: log, cfg: cfg, state: st, Applier: ap}, nil
}

func UserAgent() string {
	return AppName + "/" + AppVersion + " (+https://github.com/78tacos/dnscrypt-updater)"
}

func SetupLogger(paths config.Paths, quiet bool) (*slog.Logger, func(), error) {
	if err := paths.EnsureDir(); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	var w io.Writer = io.MultiWriter(os.Stderr, f)
	if quiet {
		w = f
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), func() { _ = f.Close() }, nil
}

// CheckOnce runs a single poll and prints the result. Exit codes:
// 0 up to date / skipped / snoozed, 1 error, 2 update available, 3 not found.
func (rt *Runtime) CheckOnce(ctx context.Context, stdout io.Writer) (int, error) {
	res, err := rt.poll(ctx, rt.Opts.ForceNotify)
	if rt.Opts.JSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if jerr := enc.Encode(res); jerr != nil {
			return 1, jerr
		}
	} else if !rt.Opts.Quiet {
		fmt.Fprintln(stdout, res.Message)
		if res.BinaryPath != "" {
			fmt.Fprintf(stdout, "binary: %s (%s)\n", res.BinaryPath, res.LocalSource)
		}
		if res.ReleaseURL != "" {
			fmt.Fprintf(stdout, "release: %s\n", res.ReleaseURL)
		}
		if res.OfficialAsset != "" {
			fmt.Fprintf(stdout, "official asset: %s\n", res.OfficialAsset)
			if res.MinisigName != "" {
				fmt.Fprintf(stdout, "minisig: %s\n", res.MinisigName)
			}
			fmt.Fprintf(stdout, "minisign pubkey: %s\n", githubrel.MinisignPubKey)
		}
		fmt.Fprintf(stdout, "source: %s (official upstream, not a fork)\n", githubrel.LatestURL)
	}
	if err != nil {
		return 1, err
	}
	// -check-once is quiet/script-friendly unless -notify is also passed.
	// Tray fallback (CheckOnce=false) still shows a desktop notification.
	if res.ShouldNotify && (!rt.Opts.CheckOnce || rt.Opts.ForceNotify) {
		rt.maybeNotify(res)
	}
	switch {
	case res.NotFound:
		return 3, nil
	case res.UpdateAvailable:
		return 2, nil
	default:
		return 0, nil
	}
}

func (rt *Runtime) poll(ctx context.Context, force bool) (check.Result, error) {
	rt.mu.Lock()
	cfg := rt.cfg
	st := rt.state
	rt.mu.Unlock()

	res, st, err := rt.Engine.Run(ctx, cfg, st, force)
	rt.mu.Lock()
	rt.state = st
	rt.last = res
	rt.mu.Unlock()
	if serr := config.SaveState(rt.Paths.State, st); serr != nil {
		rt.Log.Warn("save state", "err", serr)
	}
	if err != nil {
		rt.Log.Error("check failed", "err", err)
	} else {
		rt.Log.Info("check", "local", res.LocalVersion, "remote", res.RemoteVersion, "update", res.UpdateAvailable, "notify", res.ShouldNotify)
	}
	return res, err
}

func (rt *Runtime) maybeNotify(res check.Result) {
	sent := false
	if res.NotFound {
		if err := notify.NotFound(); err != nil {
			rt.Log.Warn("notification failed", "err", err)
			return
		}
		sent = true
	} else if res.UpdateAvailable {
		if err := notify.UpdateAvailable(res.LocalVersion, res.RemoteVersion, res.ReleaseURL); err != nil {
			rt.Log.Warn("notification failed", "err", err)
			return
		}
		sent = true
	}
	if !sent {
		return
	}
	rt.mu.Lock()
	if res.NotFound {
		rt.state.LastNotFoundNotified = time.Now()
	}
	if res.UpdateAvailable && res.RemoteVersion != "" {
		rt.state.LastNotifiedVersion = res.RemoteVersion
	}
	st := rt.state
	rt.mu.Unlock()
	if err := config.SaveState(rt.Paths.State, st); err != nil {
		rt.Log.Warn("save state after notify", "err", err)
	}
}

func (rt *Runtime) skipCurrentRemote() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	tag := rt.last.RemoteVersion
	if tag == "" {
		return errors.New("no remote version to skip")
	}
	rt.cfg.SkipVersion = tag
	return config.SaveFile(rt.Paths.File, rt.cfg)
}

func (rt *Runtime) snooze() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.cfg.SnoozeUntil = time.Now().Add(config.SnoozeDuration).UTC().Format(time.RFC3339)
	return config.SaveFile(rt.Paths.File, rt.cfg)
}

func (rt *Runtime) openRelease() error {
	rt.mu.Lock()
	u := rt.last.ReleaseURL
	rt.mu.Unlock()
	if u == "" {
		u = githubrel.ReleasePage
	}
	return openurl.OpenRelease(u)
}

func (rt *Runtime) snapshot() check.Result {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.last
}

func (rt *Runtime) interval() time.Duration {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	d, err := rt.cfg.Interval()
	if err != nil {
		rt.Log.Warn("invalid check_interval, using default", "err", err)
		return config.DefaultInterval
	}
	return d
}

func (rt *Runtime) applier() *apply.Applier {
	if rt.Applier != nil {
		return rt.Applier
	}
	rt.Applier = &apply.Applier{Log: rt.Log, UserAgent: UserAgent(), Getenv: os.Getenv}
	return rt.Applier
}

func (rt *Runtime) signedArchive() (githubrel.SignedArchive, string, error) {
	res := rt.snapshot()
	if res.OfficialAssetURL == "" || res.MinisigURL == "" {
		return githubrel.SignedArchive{}, "", errors.New("no official signed archive identified; check GitHub first")
	}
	return githubrel.SignedArchive{
		Archive: githubrel.Asset{Name: res.OfficialAsset, BrowserDownloadURL: res.OfficialAssetURL},
		Minisig: githubrel.Asset{Name: res.MinisigName, BrowserDownloadURL: res.MinisigURL},
	}, res.RemoteVersion, nil
}

// Install downloads the official signed dnscrypt-proxy archive, verifies it,
// installs it, and on Windows can register the service and set system DNS.
func (rt *Runtime) Install(ctx context.Context) (apply.Result, error) {
	if _, err := rt.poll(ctx, false); err != nil {
		return apply.Result{}, err
	}
	archive, tag, err := rt.signedArchive()
	if err != nil {
		return apply.Result{}, err
	}

	rt.mu.Lock()
	cfg := rt.cfg
	existing := rt.last.BinaryPath
	rt.mu.Unlock()

	opts := apply.Options{
		InstallDir:     firstNonEmpty(rt.Opts.InstallDir, cfg.InstallDir),
		ExistingBinary: firstNonEmpty(rt.Opts.BinaryPath, existing),
		SetSystemDNS:   cfg.SetSystemDNS && !rt.Opts.NoDNS,
		ManageService:  cfg.ManageService && !rt.Opts.NoService,
	}

	ap := rt.applier()
	if ap.NeedsWindowsElevation() {
		args := []string{"-install", "-quiet", "-config", rt.Paths.File}
		if rt.Opts.NoDNS || !cfg.SetSystemDNS {
			args = append(args, "-no-dns")
		}
		if rt.Opts.NoService || !cfg.ManageService {
			args = append(args, "-no-service")
		}
		if opts.InstallDir != "" {
			args = append(args, "-install-dir", opts.InstallDir)
		}
		rt.Log.Info("requesting administrator permission to install dnscrypt-proxy")
		code, err := ap.RelaunchElevated(args)
		if err != nil {
			return apply.Result{}, err
		}
		if code != 0 {
			return apply.Result{}, fmt.Errorf("elevated install exited %d", code)
		}
		cfg2, err := config.LoadFile(rt.Paths.File)
		if err == nil {
			rt.mu.Lock()
			rt.cfg = cfg2
			rt.mu.Unlock()
		}
		res, _ := rt.poll(ctx, false)
		out := apply.Result{
			Version:    tag,
			BinaryPath: res.BinaryPath,
			Verified:   true,
			Message:    "Installed dnscrypt-proxy with administrator permission.",
		}
		if res.BinaryPath != "" && !res.NotFound {
			out.BinaryPath = res.BinaryPath
			out.Version = res.LocalVersion
			out.Message = fmt.Sprintf("Installed dnscrypt-proxy %s at %s.", res.LocalVersion, res.BinaryPath)
		}
		return out, nil
	}

	res, err := ap.Apply(ctx, archive, tag, opts)
	if err != nil {
		_ = notify.InstallFailed(err)
		return res, err
	}
	if res.BinaryPath != "" {
		rt.mu.Lock()
		rt.cfg.BinaryPath = res.BinaryPath
		saveErr := config.SaveFile(rt.Paths.File, rt.cfg)
		rt.mu.Unlock()
		if saveErr != nil {
			rt.Log.Warn("save binary_path after install", "err", saveErr)
		}
	}
	if nerr := notify.Installed(res.Message); nerr != nil {
		rt.Log.Warn("install notification", "err", nerr)
	}
	if _, perr := rt.poll(ctx, false); perr != nil {
		rt.Log.Warn("post-install check", "err", perr)
	}
	return res, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
