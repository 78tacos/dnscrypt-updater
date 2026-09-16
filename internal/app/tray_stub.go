//go:build !windows && !systray

package app

// RunTray is a stub on Linux/macOS builds without -tags systray (and CGO).
func RunTray(*Runtime) error {
	return ErrTrayUnavailable
}
