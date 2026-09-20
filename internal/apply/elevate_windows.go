//go:build windows

package apply

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func isElevated() bool {
	cmd := exec.Command("net", "session")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run() == nil
}

func relaunchElevatedAndWait(args []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 1, err
	}
	script := "$p = Start-Process -FilePath " + psQuote(exe) +
		" -ArgumentList " + psArgList(args) +
		" -Verb RunAs -Wait -PassThru -WindowStyle Hidden; if ($null -eq $p) { exit 1 }; exit $p.ExitCode"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		code := ee.ExitCode()
		msg := strings.ToLower(string(ee.Stderr))
		if strings.Contains(msg, "cancel") || code == 1223 {
			return code, ErrElevationCancelled
		}
		// PowerShell returns ExitError for any non-zero child exit; prefer the
		// numeric code and a stable sentinel so callers can read .apply-error.txt.
		if code != 0 {
			return code, fmt.Errorf("%w: exit status %d", ErrElevationFailed, code)
		}
		return code, err
	}
	if strings.Contains(strings.ToLower(err.Error()), "cancel") {
		return 1, ErrElevationCancelled
	}
	return 1, err
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func psArgList(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = psQuote(a)
	}
	return "@(" + strings.Join(parts, ",") + ")"
}
