//go:build windows

package instance

import (
	"golang.org/x/sys/windows"
)

// Acquire creates Local\dnscrypt-proxy-updater and holds the handle until release.
// A second process gets ERROR_ALREADY_EXISTS.
func Acquire() (func(), error) {
	return acquireNamed(MutexName)
}

func acquireNamed(name string) (func(), error) {
	utf, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, utf)
	release, claimErr := claimMutex(uintptr(handle), err, func(h uintptr) {
		_ = windows.CloseHandle(windows.Handle(h))
	})
	if claimErr != nil {
		return nil, claimErr
	}
	return reusable(release), nil
}

// ShowAlreadyRunning tells the user the tray is already open. The release
// builds have no console, so a message box is the visible surface.
func ShowAlreadyRunning() {
	text, err := windows.UTF16PtrFromString(AlreadyRunningMessage)
	if err != nil {
		return
	}
	caption, err := windows.UTF16PtrFromString("dnscrypt-proxy-updater")
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONINFORMATION)
}
