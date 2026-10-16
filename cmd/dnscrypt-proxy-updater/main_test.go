package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/instance"
)

func TestTakeInteractiveLockSkipsOneShot(t *testing.T) {
	origA, origE := acquireInstance, userErr
	t.Cleanup(func() {
		acquireInstance = origA
		userErr = origE
	})
	cases := []struct {
		name      string
		install   bool
		checkOnce bool
		wantLock  bool
	}{
		{name: "tray", wantLock: true},
		{name: "quiet tray still locks", wantLock: true},
		{name: "install", install: true},
		{name: "elevated install quiet", install: true},
		{name: "check-once", checkOnce: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			acquireInstance = func() (func(), error) {
				called = true
				return func() {}, nil
			}
			release, stop, code := takeInteractiveLock(tc.install, tc.checkOnce)
			if stop || code != 0 {
				t.Fatalf("stop %v code %d", stop, code)
			}
			release()
			if called != tc.wantLock {
				t.Fatalf("locked %v want %v", called, tc.wantLock)
			}
		})
	}
}

func TestRunSecondTrayExitsCleanly(t *testing.T) {
	origA, origS, origE := acquireInstance, showAlreadyRunning, userErr
	t.Cleanup(func() {
		acquireInstance = origA
		showAlreadyRunning = origS
		userErr = origE
	})
	acquireInstance = func() (func(), error) {
		return nil, instance.ErrAlreadyRunning
	}
	shown := 0
	showAlreadyRunning = func() { shown++ }
	var buf bytes.Buffer
	userErr = &buf

	if code := run(nil); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if shown != 1 {
		t.Fatalf("shown %d", shown)
	}
	if !bytes.Contains(buf.Bytes(), []byte(instance.AlreadyRunningMessage)) {
		t.Fatalf("stderr %q", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("system tray")) {
		t.Fatalf("stderr %q", buf.String())
	}
}

func TestRunVersionSkipsLock(t *testing.T) {
	orig := acquireInstance
	t.Cleanup(func() { acquireInstance = orig })
	called := false
	acquireInstance = func() (func(), error) {
		called = true
		return func() {}, errors.New("should not lock")
	}
	if code := run([]string{"-version"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if called {
		t.Fatal("version acquired the lock")
	}
}

func TestRunQuietTrayTakesLock(t *testing.T) {
	origA, origS, origE := acquireInstance, showAlreadyRunning, userErr
	t.Cleanup(func() {
		acquireInstance = origA
		showAlreadyRunning = origS
		userErr = origE
	})
	acquireInstance = func() (func(), error) {
		return nil, instance.ErrAlreadyRunning
	}
	shown := 0
	showAlreadyRunning = func() { shown++ }
	userErr = &bytes.Buffer{}

	if code := run([]string{"-quiet"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if shown != 1 {
		t.Fatal("quiet tray did not take the single-instance lock")
	}
}

func TestRunLockFailureExits(t *testing.T) {
	origA, origE := acquireInstance, userErr
	t.Cleanup(func() {
		acquireInstance = origA
		userErr = origE
	})
	acquireInstance = func() (func(), error) {
		return nil, errors.New("mutex failed")
	}
	var buf bytes.Buffer
	userErr = &buf
	if code := run(nil); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !bytes.Contains(buf.Bytes(), []byte("mutex failed")) {
		t.Fatalf("stderr %q", buf.String())
	}
}

func TestRunInstallDoesNotLock(t *testing.T) {
	orig := acquireInstance
	t.Cleanup(func() { acquireInstance = orig })
	called := false
	acquireInstance = func() (func(), error) {
		called = true
		return func() {}, errors.New("should not lock")
	}
	release, stop, code := takeInteractiveLock(true, false)
	if called || stop || code != 0 {
		t.Fatalf("install lock called %v stop %v code %d", called, stop, code)
	}
	release()

	release, stop, code = takeInteractiveLock(true, false)
	if called {
		t.Fatal("-install -quiet would be blocked")
	}
	release()
}
