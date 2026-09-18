package notify

import "github.com/gen2brain/beeep"

// Send shows a desktop notification. Tests may replace this.
var Send = beeepNotify

func beeepNotify(title, message, iconPath string) error {
	return beeep.Notify(title, message, iconPath)
}

func UpdateAvailable(local, remote, releaseURL string) error {
	title := "dnscrypt-proxy update available"
	msg := local + " → " + remote + "\nOfficial release: " + releaseURL + "\nUse Install / Update in the tray, or run with -install."
	return Send(title, msg, "")
}

func NotFound() error {
	return Send(
		"dnscrypt-proxy not found",
		"dnscrypt-proxy-updater could not run dnscrypt-proxy -version. Use Install in the tray, or: dnscrypt-proxy-updater -install",
		"",
	)
}

func Installed(message string) error {
	if message == "" {
		message = "dnscrypt-proxy was installed."
	}
	return Send("dnscrypt-proxy installed", message, "")
}

func InstallFailed(err error) error {
	msg := "install failed"
	if err != nil {
		msg = err.Error()
	}
	return Send("dnscrypt-proxy install failed", msg, "")
}
