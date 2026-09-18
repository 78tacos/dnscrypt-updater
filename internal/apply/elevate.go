package apply

import "errors"

var (
	// ErrElevationUnsupported is returned when automatic UAC relaunch is not available.
	ErrElevationUnsupported = errors.New("automatic elevation is only supported on Windows")
	// ErrElevationCancelled is returned when the user dismisses the UAC prompt.
	ErrElevationCancelled = errors.New("administrator permission was declined")
)
