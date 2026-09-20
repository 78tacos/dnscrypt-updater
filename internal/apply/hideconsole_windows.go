//go:build windows

package apply

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents a console window from appearing for console-subsystem
// children (dnscrypt-proxy.exe, sc.exe, powershell, net, …).
const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
