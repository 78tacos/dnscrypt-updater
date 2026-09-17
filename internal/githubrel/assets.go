package githubrel

import (
	"strings"
)

const (
	// Homepage is the project site (distinct from unofficial dnscrypt.org client downloads).
	Homepage = "https://dnscrypt.info"

	// MinisignPubKey verifies official GitHub release archives.
	// Also published as DNSSEC TXT dnscrypt-proxy.key.dnscrypt.info.
	MinisignPubKey = "RWTk1xXqcTODeYttYMCMLo0YJHaFEHn7a3akqHlb/7QvIQXHVPxKbjB5"

	// MinisignDNS is the DNSSEC-signed TXT that publishes MinisignPubKey.
	MinisignDNS = "dnscrypt-proxy.key.dnscrypt.info"
)

// Asset is one GitHub release file.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// SignedArchive is an official OS/arch archive plus its matching .minisig.
type SignedArchive struct {
	Archive Asset
	Minisig Asset
}

// PlatformToken is the identifier used in official asset names
// (win64, linux_x86_64, macos_arm64, …).
func PlatformToken(goos, goarch string) string {
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return "win64"
		case "386":
			return "win32"
		case "arm64":
			return "winarm"
		default:
			return ""
		}
	case "linux":
		switch goarch {
		case "amd64":
			return "linux_x86_64"
		case "386":
			return "linux_i386"
		case "arm64":
			return "linux_arm64"
		case "arm":
			return "linux_arm"
		case "riscv64":
			return "linux_riscv64"
		case "mips":
			return "linux_mips"
		case "mipsle":
			return "linux_mipsle"
		case "mips64":
			return "linux_mips64"
		case "mips64le":
			return "linux_mips64le"
		case "loong64":
			return "linux_loong64"
		default:
			return ""
		}
	case "darwin":
		switch goarch {
		case "amd64":
			return "macos_x86_64"
		case "arm64":
			return "macos_arm64"
		default:
			return ""
		}
	case "freebsd":
		return bsdToken("freebsd", goarch)
	case "netbsd":
		return bsdToken("netbsd", goarch)
	case "openbsd":
		return bsdToken("openbsd", goarch)
	case "dragonfly":
		if goarch == "amd64" {
			return "dragonflybsd_amd64"
		}
		return ""
	case "solaris", "illumos":
		if goarch == "amd64" {
			return "solaris_amd64"
		}
		return ""
	case "android":
		switch goarch {
		case "amd64":
			return "android_x86_64"
		case "386":
			return "android_i386"
		case "arm64":
			return "android_arm64"
		case "arm":
			return "android_arm"
		default:
			return ""
		}
	default:
		return ""
	}
}

func bsdToken(os, goarch string) string {
	switch goarch {
	case "amd64":
		return os + "_amd64"
	case "386":
		return os + "_i386"
	case "arm64":
		return os + "_arm64"
	case "arm":
		return os + "_arm"
	default:
		return ""
	}
}

// SelectSignedArchive picks the official zip/tar.gz for this OS/arch and its .minisig.
// MSI installers are ignored: they are not minisign-signed on the GitHub release.
func SelectSignedArchive(goos, goarch string, assets []Asset) (SignedArchive, bool) {
	token := PlatformToken(goos, goarch)
	if token == "" {
		return SignedArchive{}, false
	}
	needle := "dnscrypt-proxy-" + token + "-"
	var archive Asset
	for _, a := range assets {
		if !strings.HasPrefix(a.Name, needle) {
			continue
		}
		if strings.HasSuffix(a.Name, ".minisig") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(a.Name), ".msi") {
			continue
		}
		if strings.HasSuffix(a.Name, ".zip") || strings.HasSuffix(a.Name, ".tar.gz") {
			archive = a
			break
		}
	}
	if archive.Name == "" {
		return SignedArchive{}, false
	}
	sigName := archive.Name + ".minisig"
	for _, a := range assets {
		if a.Name == sigName {
			return SignedArchive{Archive: archive, Minisig: a}, true
		}
	}
	return SignedArchive{}, false
}
