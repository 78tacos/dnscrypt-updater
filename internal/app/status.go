package app

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/78tacos/dnscrypt-updater/internal/check"
)

//go:embed icon.ico
var iconICO []byte

//go:embed icon.png
var iconPNG []byte

func tooltipFor(res check.Result) string {
	switch {
	case res.RemoteFetchError != "" && res.RemoteVersion == "":
		return "dnscrypt-updater: GitHub check failed"
	case res.NotFound:
		return "dnscrypt-updater: dnscrypt-proxy not found"
	case res.UpdateAvailable:
		return fmt.Sprintf("dnscrypt-updater: %s available (local %s)", res.RemoteVersion, res.LocalVersion)
	case res.LocalVersion != "" && res.RemoteVersion != "":
		return fmt.Sprintf("dnscrypt-updater: up to date (%s)", res.LocalVersion)
	default:
		return "dnscrypt-updater"
	}
}

func statusMenuTitle(res check.Result) string {
	if res.NotFound {
		return "Local: not found"
	}
	src := res.LocalSource
	if src == "" {
		src = "unknown"
	}
	return fmt.Sprintf("Local %s (%s)", emptyDash(res.LocalVersion), src)
}

func remoteMenuTitle(res check.Result) string {
	if res.RemoteVersion == "" {
		return "GitHub: (unavailable)"
	}
	if res.UpdateAvailable {
		return fmt.Sprintf("GitHub %s — update available", res.RemoteVersion)
	}
	return fmt.Sprintf("GitHub %s", res.RemoteVersion)
}

func assetMenuTitle(res check.Result) string {
	if res.OfficialAsset == "" {
		return "Official asset: (see GitHub release)"
	}
	return "Asset: " + res.OfficialAsset + " + .minisig"
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
