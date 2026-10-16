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

func SettingsQueued(pendingDir string) error {
	msg := "Could not write the install directory. Settings are saved in " + pendingDir + ". Right-click the tray and choose Apply pending settings (Administrator may be required)."
	return Send("dnscrypt-proxy settings queued", msg, "")
}

func SettingsApplied(message string) error {
	if message == "" {
		message = "Pending settings were applied."
	}
	return Send("dnscrypt-proxy settings applied", message, "")
}

func SettingsApplyFailed(err error) error {
	msg := "could not apply pending settings"
	if err != nil {
		msg = err.Error()
	}
	return Send("dnscrypt-proxy settings apply failed", msg, "")
}
