package apply

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
)

const elevationCancelledErrno = syscall.Errno(1223) // ERROR_CANCELLED

var (
	// ErrElevationUnsupported is returned when automatic UAC relaunch is not available.
	ErrElevationUnsupported = errors.New("automatic elevation is only supported on Windows")
	// ErrElevationCancelled is returned when the user dismisses the UAC prompt.
	ErrElevationCancelled = errors.New("administrator permission was declined")
	// ErrElevationFailed is returned when an elevated child exits non-zero.
	ErrElevationFailed = errors.New("elevated process failed")
)

// shellLauncher starts verb against file (ShellExecute "runas") and waits for the process.
// params is a Windows command line, not a raw argument slice.
type shellLauncher func(verb, file, params string) (int, error)

// launchElevated relaunches exe via ShellExecute verb "runas" and waits for it.
// A declined UAC prompt (ERROR_CANCELLED 1223) is ErrElevationCancelled.
func launchElevated(exe string, args []string, run shellLauncher) (int, error) {
	if strings.TrimSpace(exe) == "" {
		return 1, errors.New("elevation executable path is empty")
	}
	if run == nil {
		return 1, ErrElevationUnsupported
	}
	code, err := run("runas", exe, windowsCommandLine(args))
	if err != nil {
		if elevationCancelled(err) {
			return 1223, ErrElevationCancelled
		}
		return 1, fmt.Errorf("could not request administrator permission: %w", err)
	}
	return code, nil
}

func elevationCancelled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrElevationCancelled) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == elevationCancelledErrno
}

// windowsCommandLine quotes args for ShellExecute lpParameters / CommandLineToArgvW.
func windowsCommandLine(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = escapeWindowsArg(arg)
	}
	return strings.Join(parts, " ")
}

// escapeWindowsArg follows the CommandLineToArgvW quoting rules.
func escapeWindowsArg(s string) string {
	if s == "" {
		return `""`
	}
	needsQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '"':
			needsQuote = true
		}
	}
	if !needsQuote {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			slashes++
			b.WriteByte('\\')
		case '"':
			for ; slashes > 0; slashes-- {
				b.WriteByte('\\')
			}
			b.WriteString(`\"`)
		default:
			slashes = 0
			b.WriteByte(s[i])
		}
	}
	for ; slashes > 0; slashes-- {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}
