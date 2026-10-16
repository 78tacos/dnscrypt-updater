// Package instance keeps a single long-running tray process.
package instance

import (
	"errors"
	"sync"
	"syscall"
)

const (
	// MutexName is the per-session Windows mutex. Local\ matches a tray icon,
	// which lives in one interactive session. Global\ would block other logons.
	MutexName = `Local\dnscrypt-proxy-updater`

	// AlreadyRunningMessage is shown when a second interactive launch finds the first.
	AlreadyRunningMessage = "dnscrypt-updater is already running. Look for the icon in the system tray."
)

// ErrAlreadyRunning is returned when another interactive instance holds the lock.
var ErrAlreadyRunning = errors.New("dnscrypt-updater is already running")

// ERROR_ALREADY_EXISTS from CreateMutex.
const errorAlreadyExists = syscall.Errno(183)

// Interactive reports whether this process is the long-running tray.
// Install, -check-once, and other one-shot runs (including the elevated
// -install -quiet relaunch) do not take the lock.
func Interactive(install, checkOnce bool) bool {
	return !SkipsLock(install, checkOnce, false, false)
}

// SkipsLock reports commands that must not take the tray mutex.
// -apply-config is the elevated ShellExecuteEx child started while the tray
// still holds the lock. -apply-pending is the same kind of one-shot.
func SkipsLock(install, checkOnce, applyConfig, applyPending bool) bool {
	return install || checkOnce || applyConfig || applyPending
}

func reusable(release func()) func() {
	if release == nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(release) }
}

// claimMutex turns a CreateMutex result into a held lock or ErrAlreadyRunning.
// The duplicate handle from ERROR_ALREADY_EXISTS is closed.
func claimMutex(handle uintptr, err error, closeHandle func(uintptr)) (func(), error) {
	if errors.Is(err, errorAlreadyExists) {
		if handle != 0 && closeHandle != nil {
			closeHandle(handle)
		}
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		if handle != 0 && closeHandle != nil {
			closeHandle(handle)
		}
		return nil, err
	}
	if handle == 0 {
		return nil, errors.New("single-instance mutex was not created")
	}
	return func() {
		if closeHandle != nil {
			closeHandle(handle)
		}
	}, nil
}
