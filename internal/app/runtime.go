package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/78tacos/dnscrypt-updater/internal/check"
	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/detect"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
	"github.com/78tacos/dnscrypt-updater/internal/notify"
	"github.com/78tacos/dnscrypt-updater/internal/openurl"
)

const (
	AppVersion = "0.1.0"
	AppName    = "dnscrypt-updater"
)

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
	return &Runtime{Opts: opts, Paths: paths, Engine: eng, Log: log, cfg: cfg, state: st}, nil
}

func UserAgent() string {
	return "dnscrypt-updater/" + AppVersion + " (+https://github.com/78tacos/dnscrypt-updater)"
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
