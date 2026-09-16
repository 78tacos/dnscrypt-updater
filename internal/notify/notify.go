package notify

import "github.com/gen2brain/beeep"

// Send shows a desktop notification. Tests may replace this.
var Send = beeepNotify

func beeepNotify(title, message, iconPath string) error {
	return beeep.Notify(title, message, iconPath)
}

// UpdateAvailable is the v1 notify-only message. The user must open the
// GitHub release page themselves (tray action / URL in the body). This helper
// never installs or replaces dnscrypt-proxy.
func UpdateAvailable(local, remote, releaseURL string) error {
	title := "dnscrypt-proxy update available"
	msg := local + " → " + remote + "\nOfficial release: " + releaseURL + "\nNotify-only: this app will not install the update."
	return Send(title, msg, "")
}

func NotFound() error {
	return Send(
		"dnscrypt-proxy not found",
		"dnscrypt-updater could not run dnscrypt-proxy -version. Set binary_path or current_version in config.json. No version was assumed.",
		"",
	)
}
