//go:build !windows

package autostart

import "os"

func currentUserDirs() (home, xdg string) {
	home, _ = os.UserHomeDir()
	return home, os.Getenv("XDG_CONFIG_HOME")
}

func currentRegistry() RegistryStore { return nil }
