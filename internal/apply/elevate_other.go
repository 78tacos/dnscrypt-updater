//go:build !windows

package apply

import (
	"os"
)

func isElevated() bool {
	return os.Geteuid() == 0
}

func relaunchElevatedAndWait([]string) (int, error) {
	return 1, ErrElevationUnsupported
}
