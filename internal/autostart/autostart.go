// Package autostart turns on dnscrypt-proxy-updater at login without administrator rights.
// Windows writes HKCU\...\Run, Linux writes an XDG autostart desktop file, and
// macOS writes a LaunchAgent plist with RunAtLoad.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// AppName is the login-item name (registry value, desktop file, plist label stem).
	AppName = "dnscrypt-proxy-updater"
	// LaunchAgentLabel is the macOS LaunchAgent label and plist file name.
	LaunchAgentLabel = "com.78tacos.dnscrypt-proxy-updater"
	// RunKey is the per-user Windows Run key. It does not need UAC.
	RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
)

// Toggle is the start-with-the-OS control. Tests use fakes and temp directories
// instead of the real registry or the real home directory.
type Toggle interface {
	Label() string
	Help() string
	Enabled() (bool, error)
	Set(enabled bool) error
}

// RegistryStore is the Windows Run key. Production uses HKCU; tests use a map.
type RegistryStore interface {
	Get(name string) (value string, ok bool, err error)
	Set(name, value string) error
	Delete(name string) error
}

// Config selects an OS backend. Empty AppName defaults to AppName.
type Config struct {
	AppName       string
	ExecPath      string
	Home          string
	XDGConfigHome string
	Registry      RegistryStore
}

// Label is the checkbox text for goos.
func Label(goos string) string {
	switch goos {
	case "windows":
		return "Start with Windows"
	case "linux":
		return "Start with Linux"
	case "darwin":
		return "Start at login"
	default:
		return "Start with the system"
	}
}

// Help describes where the login item is stored.
func Help(goos string) string {
	switch goos {
	case "windows":
		return `Adds or removes a value under HKCU\Software\Microsoft\Windows\CurrentVersion\Run. No administrator prompt.`
	case "linux":
		return "Adds or removes ~/.config/autostart/dnscrypt-proxy-updater.desktop (honors XDG_CONFIG_HOME)."
	case "darwin":
		return "Adds or removes a LaunchAgent plist in ~/Library/LaunchAgents with RunAtLoad. It applies at the next login."
	default:
		return "Start dnscrypt-proxy-updater when you sign in."
	}
}

// ForOS builds a toggle for goos. It does not touch the real system except
// through cfg (a temp directory or a fake registry).
func ForOS(goos string, cfg Config) (Toggle, error) {
	if strings.TrimSpace(cfg.AppName) == "" {
		cfg.AppName = AppName
	}
	switch goos {
	case "windows":
		if cfg.Registry == nil {
			return nil, errors.New("windows autostart requires a registry store")
		}
		return windowsToggle{
			label: Label(goos),
			help:  Help(goos),
			name:  cfg.AppName,
			exec:  cfg.ExecPath,
			reg:   cfg.Registry,
		}, nil
	case "linux":
		if strings.TrimSpace(cfg.Home) == "" && strings.TrimSpace(cfg.XDGConfigHome) == "" {
			return nil, errors.New("linux autostart requires a home directory or XDG_CONFIG_HOME")
		}
		return linuxToggle{
			label: Label(goos),
			help:  Help(goos),
			path:  LinuxAutostartPath(cfg.Home, cfg.XDGConfigHome, cfg.AppName),
			exec:  cfg.ExecPath,
		}, nil
	case "darwin":
		if strings.TrimSpace(cfg.Home) == "" {
			return nil, errors.New("macOS autostart requires a home directory")
		}
		return darwinToggle{
			label: Label(goos),
			help:  Help(goos),
			path:  DarwinLaunchAgentPath(cfg.Home),
			exec:  cfg.ExecPath,
		}, nil
	default:
		return nil, fmt.Errorf("start-with-OS is not supported on %s", goos)
	}
}

// Current is the toggle for this process and this OS.
func Current() (Toggle, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}
	home, xdg := currentUserDirs()
	return ForOS(runtime.GOOS, Config{
		ExecPath:      exe,
		Home:          home,
		XDGConfigHome: xdg,
		Registry:      currentRegistry(),
	})
}

// LinuxAutostartPath is $XDG_CONFIG_HOME/autostart/<app>.desktop, or
// ~/.config/autostart/<app>.desktop when XDG_CONFIG_HOME is empty.
func LinuxAutostartPath(home, xdg, app string) string {
	if strings.TrimSpace(app) == "" {
		app = AppName
	}
	base := strings.TrimSpace(xdg)
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "autostart", app+".desktop")
}

// DarwinLaunchAgentPath is ~/Library/LaunchAgents/<label>.plist.
func DarwinLaunchAgentPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist")
}

// FormatRunCommand quotes a Windows executable path when it contains spaces.
func FormatRunCommand(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, `"`) && strings.HasSuffix(path, `"`) && len(path) >= 2 {
		return path
	}
	if strings.ContainsAny(path, " \t") {
		return `"` + path + `"`
	}
	return path
}
