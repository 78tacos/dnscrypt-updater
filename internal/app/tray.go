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
	systray.SetTooltip(AppName)
	systray.SetTitle(AppName)

	mTitle := systray.AddMenuItem(AppName+" "+AppVersion, "Downloads official dnscrypt-proxy and can install it for this system")
	mTitle.Disable()
	mLocal := systray.AddMenuItem("Local: checking…", "")
	mLocal.Disable()
	mRemote := systray.AddMenuItem("GitHub: checking…", "")
	mRemote.Disable()
	mAsset := systray.AddMenuItem("Official asset: checking…", "Minisign-signed GitHub archive")
	mAsset.Disable()
	systray.AddSeparator()
	mCheck := systray.AddMenuItem("Check now", "Poll official DNSCrypt/dnscrypt-proxy releases")
	mInstall := systray.AddMenuItem("Install dnscrypt-proxy", "Download, minisign-verify, and install for this system")
	mOpen := systray.AddMenuItem("Open GitHub release page", "Open the official upstream release")
	mSkip := systray.AddMenuItem("Skip this version", "Do not notify again for the current GitHub tag")
	mSnooze := systray.AddMenuItem("Snooze 24 hours", "Suppress notifications for a day")
	systray.AddSeparator()
	mSettings := systray.AddMenuItem("Configure dnscrypt-proxy…", "Open the local settings UI")
	mQuit := systray.AddMenuItem("Quit", "Quit "+AppName)

	applyStatus := func() {
		res := rt.snapshot()
		systray.SetTooltip(tooltipFor(res))
		mLocal.SetTitle(statusMenuTitle(res))
		mRemote.SetTitle(remoteMenuTitle(res))
		mAsset.SetTitle(assetMenuTitle(res))
		mInstall.SetTitle(installMenuTitle(res))
		if res.ReleaseURL == "" {
			mOpen.Disable()
		} else {
			mOpen.Enable()
		}
		if res.OfficialAssetURL == "" {
			mInstall.Disable()
		} else {
			mInstall.Enable()
		}
	}

	go func() {
		run := func(force bool) {
			res, err := rt.poll(ctx, force)
			if err != nil {
				applyStatus()
				return
			}
			if res.ShouldNotify {
				rt.maybeNotify(res)
			}
			applyStatus()
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
				applyStatus()
			case <-mInstall.ClickedCh:
				mInstall.Disable()
				mInstall.SetTitle("Installing dnscrypt-proxy…")
				res, err := rt.Install(ctx)
				if err != nil {
					rt.Log.Warn("install", "err", err)
				} else {
					rt.Log.Info("install", "msg", res.Message, "path", res.BinaryPath)
				}
				applyStatus()
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
			case <-mSettings.ClickedCh:
				if _, err := rt.OpenSettings(ctx); err != nil {
					rt.Log.Warn("settings ui", "err", err)
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
