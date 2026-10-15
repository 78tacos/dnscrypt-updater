//go:build windows

package apply

import "testing"

func TestShellExecuteInfoMatchesWindowsABI(t *testing.T) {
	t.Parallel()
	if !shellExecuteInfoLayoutOK() {
		t.Fatal("SHELLEXECUTEINFOW layout does not match the Windows ABI")
	}
}
