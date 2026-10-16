//go:build windows

package instance

import (
	"errors"
	"os"
	"strconv"
	"testing"
)

func TestAcquireNamedRejectsSecondHolder(t *testing.T) {
	name := `Local\dnscrypt-proxy-updater-test-` + strconv.Itoa(os.Getpid())
	if name == MutexName {
		t.Fatal("test mutex must not be the application mutex")
	}
	release, err := acquireNamed(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	_, err = acquireNamed(name)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire err %v", err)
	}

	release()
	release()
	again, err := acquireNamed(name)
	if err != nil {
		t.Fatal(err)
	}
	again()
}
