//go:build !windows

package instance

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func defaultLockFile() string {
	return filepath.Join(os.TempDir(), "dnscrypt-proxy-updater.lock")
}

// Acquire takes an exclusive non-blocking lock file. The lock drops when the
// process exits, including a crash.
func Acquire() (func(), error) {
	return AcquireFile(defaultLockFile())
}

// AcquireFile locks path. Tests use a private file so they do not share the
// machine-wide lock.
func AcquireFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return reusable(func() { _ = f.Close() }), nil
}

// ShowAlreadyRunning is the platform dialog. This OS has none; the caller
// writes AlreadyRunningMessage itself.
func ShowAlreadyRunning() {}
