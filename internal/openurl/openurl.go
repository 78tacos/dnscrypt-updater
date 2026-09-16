package openurl

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// ValidateReleaseURL checks that raw is an https://github.com/DNSCrypt/dnscrypt-proxy URL.
func ValidateReleaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("refusing non-https url")
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", fmt.Errorf("refusing non-github host %q", u.Host)
	}
	if !strings.HasPrefix(u.Path, "/DNSCrypt/dnscrypt-proxy") {
		return "", fmt.Errorf("refusing url outside official DNSCrypt/dnscrypt-proxy repo")
	}
	return u.String(), nil
}

// OpenRelease opens an official DNSCrypt/dnscrypt-proxy GitHub URL in the
// default browser. Non-https or non-upstream paths are rejected.
func OpenRelease(raw string) error {
	u, err := ValidateReleaseURL(raw)
	if err != nil {
		return err
	}
	return open(u)
}

func open(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	case "linux", "freebsd", "openbsd", "netbsd":
		cmd = exec.Command("xdg-open", u)
	default:
		return fmt.Errorf("open url: unsupported OS %s", runtime.GOOS)
	}
	return cmd.Start()
}
