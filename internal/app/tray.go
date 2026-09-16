//go:build windows || systray

package app

import (
	"context"
	"time"

	"github.com/getlantern/systray"
)

func trayIcon() []byte {
	if len(iconICO) > 0 {
		return iconICO
	}
	return iconPNG
}

// RunTray blocks in the system-tray event loop until Quit.
func RunTray(rt *Runtime) error {
	ctx, cancel := context.WithCancel(context.Background())
	rt.cancel = cancel
	systray.Run(func() { rt.onReady(ctx) }, func() { cancel() })
	return nil
}

func (rt *Runtime) onReady(ctx context.Context) {
	systray.SetIcon(trayIcon())
	systray.SetTooltip("dnscrypt-updater")
	systray.SetTitle("dnscrypt-updater")

	mTitle := systray.AddMenuItem("dnscrypt-updater "+AppVersion+" — notify only", "Does not install or replace dnscrypt-proxy")
	mTitle.Disable()
	mLocal := systray.AddMenuItem("Local: checking…", "")
	mLocal.Disable()
	mRemote := systray.AddMenuItem("GitHub: checking…", "")
	mRemote.Disable()
	systray.AddSeparator()
	mCheck := systray.AddMenuItem("Check now", "Poll official DNSCrypt/dnscrypt-proxy releases")
	mOpen := systray.AddMenuItem("Open GitHub release page", "Open the official upstream release (notify-only)")
	mSkip := systray.AddMenuItem("Skip this version", "Do not notify again for the current GitHub tag")
	mSnooze := systray.AddMenuItem("Snooze 24 hours", "Suppress notifications for a day")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit dnscrypt-updater")

	apply := func() {
		res := rt.snapshot()
		systray.SetTooltip(tooltipFor(res))
		mLocal.SetTitle(statusMenuTitle(res))
		mRemote.SetTitle(remoteMenuTitle(res))
		if res.ReleaseURL == "" {
			mOpen.Disable()
		} else {
			mOpen.Enable()
		}
	}

	go func() {
		// Initial check at login/start, then on the configured interval.
		run := func(force bool) {
			res, err := rt.poll(ctx, force)
			if err != nil {
				apply()
				return
			}
			if res.ShouldNotify {
				rt.maybeNotify(res)
			}
			apply()
		}
		run(false)
		for {
			d := rt.interval()
			timer := time.NewTimer(d)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				run(false)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-mCheck.ClickedCh:
				res, _ := rt.poll(ctx, true)
				if res.ShouldNotify {
					rt.maybeNotify(res)
				}
				apply()
			case <-mOpen.ClickedCh:
				if err := rt.openRelease(); err != nil {
					rt.Log.Warn("open release", "err", err)
				}
			case <-mSkip.ClickedCh:
				if err := rt.skipCurrentRemote(); err != nil {
					rt.Log.Warn("skip", "err", err)
				} else {
					rt.Log.Info("skipped version", "tag", rt.snapshot().RemoteVersion)
				}
			case <-mSnooze.ClickedCh:
				if err := rt.snooze(); err != nil {
					rt.Log.Warn("snooze", "err", err)
				} else {
					rt.Log.Info("snoozed 24h")
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
