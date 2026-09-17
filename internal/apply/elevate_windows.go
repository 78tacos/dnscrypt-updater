//go:build windows

package apply

import (
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
		msg := strings.ToLower(string(ee.Stderr))
		if strings.Contains(msg, "cancel") || ee.ExitCode() == 1223 {
			return ee.ExitCode(), ErrElevationCancelled
		}
		return ee.ExitCode(), err
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
