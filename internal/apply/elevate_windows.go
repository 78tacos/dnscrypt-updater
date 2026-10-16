//go:build windows

package apply

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isElevated() bool {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// relaunchElevatedAndWait shows the UAC consent dialog via ShellExecuteEx "runas"
// and blocks until the elevated process exits.
func relaunchElevatedAndWait(args []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 1, err
	}
	return launchElevated(exe, args, shellExecuteRunAs)
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	swShowNormal          = 1
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

// shellExecuteInfo matches SHELLEXECUTEINFOW. Field alignment follows the
// Windows ABI (112 bytes on 64-bit, 60 on 32-bit).
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       windows.Handle
}

func shellExecuteRunAs(verb, file, params string) (int, error) {
	verbPtr, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return 1, err
	}
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 1, err
	}
	var paramsPtr *uint16
	if params != "" {
		paramsPtr, err = windows.UTF16PtrFromString(params)
		if err != nil {
			return 1, err
		}
	}
	sei := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		lpVerb:       verbPtr,
		lpFile:       filePtr,
		lpParameters: paramsPtr,
		nShow:        swShowNormal,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))
	r1, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if r1 == 0 {
		if callErr == nil {
			return 1, fmt.Errorf("ShellExecuteEx %s failed", verb)
		}
		return 1, callErr
	}
	if sei.hProcess == 0 {
		return 0, nil
	}
	defer windows.CloseHandle(sei.hProcess)
	if _, err := windows.WaitForSingleObject(sei.hProcess, windows.INFINITE); err != nil {
		return 1, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(sei.hProcess, &code); err != nil {
		return 1, err
	}
	return int(code), nil
}

func shellExecuteInfoLayoutOK() bool {
	var sei shellExecuteInfo
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return unsafe.Sizeof(sei) == 112 && unsafe.Offsetof(sei.hProcess) == 104
	case "386", "arm":
		return unsafe.Sizeof(sei) == 60 && unsafe.Offsetof(sei.hProcess) == 56
	default:
		return unsafe.Sizeof(sei) > 0
	}
}
