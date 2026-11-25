//go:build windows

package autostart

import "os"

func currentUserDirs() (home, xdg string) {
	home, _ = os.UserHomeDir()
	return home, ""
}

func currentRegistry() RegistryStore { return systemRegistry{} }
