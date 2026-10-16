package instance

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestInteractiveOnlyForTray(t *testing.T) {
	t.Parallel()
	if !Interactive(false, false) {
		t.Fatal("tray should take the lock")
	}
	for _, tc := range []struct {
		name      string
		install   bool
		checkOnce bool
	}{
		{name: "install", install: true},
		{name: "elevated install", install: true},
		{name: "check-once", checkOnce: true},
		{name: "check-once and install", install: true, checkOnce: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if Interactive(tc.install, tc.checkOnce) {
				t.Fatal("one-shot run took the lock")
			}
		})
	}
}

func TestAlreadyRunningMessagePointsAtTray(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"dnscrypt-updater is already running", "system tray"} {
		if !strings.Contains(AlreadyRunningMessage, want) {
			t.Fatalf("message %q missing %q", AlreadyRunningMessage, want)
		}
	}
	if ErrAlreadyRunning.Error() != "dnscrypt-updater is already running" {
		t.Fatalf("error %q", ErrAlreadyRunning)
	}
	if MutexName != `Local\dnscrypt-proxy-updater` {
		t.Fatalf("mutex %q", MutexName)
	}
	if strings.Contains(MutexName, `Global\`) {
		t.Fatal("mutex must stay in the interactive session")
	}
}

func TestClaimMutexAlreadyExistsClosesDuplicate(t *testing.T) {
	t.Parallel()
	var closed uintptr
	release, err := claimMutex(42, syscall.Errno(183), func(h uintptr) { closed = h })
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err %v", err)
	}
	if release != nil {
		t.Fatal("duplicate holder returned a release func")
	}
	if closed != 42 {
		t.Fatalf("closed %d", closed)
	}
}

func TestClaimMutexHoldsHandle(t *testing.T) {
	t.Parallel()
	var closed uintptr
	release, err := claimMutex(7, nil, func(h uintptr) { closed = h })
	if err != nil {
		t.Fatal(err)
	}
	release()
	if closed != 7 {
		t.Fatalf("closed %d", closed)
	}
}

func TestClaimMutexCreateFailure(t *testing.T) {
	t.Parallel()
	_, err := claimMutex(0, errors.New("access denied"), func(uintptr) {})
	if err == nil || errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err %v", err)
	}
}
