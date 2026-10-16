//go:build !windows

package instance

import (
	"errors"
	"testing"
)

func TestAcquireFileRejectsSecondHolder(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/single.lock"
	release, err := AcquireFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	_, err = AcquireFile(path)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire err %v", err)
	}

	release()
	release()
	again, err := AcquireFile(path)
	if err != nil {
		t.Fatal(err)
	}
	again()
}
