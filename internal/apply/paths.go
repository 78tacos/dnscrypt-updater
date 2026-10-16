package apply

import (
	"path/filepath"
	"runtime"
	"strings"
)

func (a *Applier) goos() string {
	if a != nil && a.GOOS != "" {
		return a.GOOS
	}
	return runtime.GOOS
}

func (a *Applier) goarch() string {
	if a != nil && a.GOARCH != "" {
		return a.GOARCH
	}
	return runtime.GOARCH
}

func (a *Applier) getenv(key string) string {
	if a != nil && a.Getenv != nil {
		return a.Getenv(key)
	}
	return ""
}

// ProxyBinaryName is dnscrypt-proxy.exe on Windows and dnscrypt-proxy elsewhere.
func ProxyBinaryName(goos string) string {
	return proxyBinaryName(goos)
}

func proxyBinaryName(goos string) string {
	if goos == "windows" {
		return "dnscrypt-proxy.exe"
	}
	return "dnscrypt-proxy"
}

// DefaultInstallDir is Program Files\dnscrypt-proxy on Windows and /opt/dnscrypt-proxy elsewhere.
func DefaultInstallDir(goos string, getenv func(string) string) string {
	if goos == "windows" {
		if getenv != nil {
			if pf := getenv("ProgramFiles"); pf != "" {
				return filepath.Join(pf, "dnscrypt-proxy")
			}
		}
		return filepath.Join(`C:\Program Files`, "dnscrypt-proxy")
	}
	return "/opt/dnscrypt-proxy"
}

func (a *Applier) defaultInstallDir() string {
	return DefaultInstallDir(a.goos(), a.getenv)
}

func packageManagedPath(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, `\`, `/`))
	markers := []string{
		"/scoop/apps/",
		"/chocolatey/",
		"chocolatey/bin",
		"/homebrew/",
		"/opt/homebrew/",
		"/usr/local/cellar/",
		"/usr/bin/",
		"/usr/sbin/",
	}
	for _, m := range markers {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

func resolveInstallDir(configured, existingBinary, goos string, getenv func(string) string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	if existingBinary != "" && !packageManagedPath(existingBinary) {
		return filepath.Dir(existingBinary)
	}
	return DefaultInstallDir(goos, getenv)
}
