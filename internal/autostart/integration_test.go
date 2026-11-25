package autostart

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHKCURunIntegration writes a throwaway value under the real
// HKCU\...\Run key, reads it back, and deletes it. It skips on every OS
// except Windows. The value name is not the production login-item name.
func TestHKCURunIntegration(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("HKCU Run integration runs on Windows")
	}
	reg := realRegistry()
	if reg == nil {
		t.Fatal("windows registry backend is missing")
	}
	name := ciRunValueName(t)
	t.Cleanup(func() {
		if err := reg.Delete(name); err != nil {
			t.Errorf("cleanup HKCU Run value %s: %v", name, err)
		}
	})
	if err := reg.Delete(name); err != nil {
		t.Fatal(err)
	}

	exec := `C:\Program Files\dnscrypt-proxy-updater-ci.exe`
	tog, err := ForOS("windows", Config{AppName: name, ExecPath: exec, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	on, err := tog.Enabled()
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatalf("value %s already enabled", name)
	}

	if err := tog.Set(true); err != nil {
		t.Fatal(err)
	}
	got, ok, err := reg.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("HKCU Run value %s was not written", name)
	}
	if want := FormatRunCommand(exec); got != want {
		t.Fatalf("HKCU Run value %q want %q", got, want)
	}
	on, err = tog.Enabled()
	if err != nil || !on {
		t.Fatalf("enabled=%v err=%v", on, err)
	}

	if err := tog.Set(false); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := reg.Get(name); err != nil || ok {
		t.Fatalf("value remains ok=%v err=%v", ok, err)
	}
	on, err = tog.Enabled()
	if err != nil || on {
		t.Fatalf("after disable enabled=%v err=%v", on, err)
	}
}

// TestLaunchAgentTempHomeIntegration writes, reads, and removes a LaunchAgent
// plist under a temporary HOME. It skips on every OS except macOS.
func TestLaunchAgentTempHomeIntegration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("LaunchAgent integration runs on macOS")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	exec := "/usr/bin/true"
	tog, err := ForOS("darwin", Config{Home: home, ExecPath: exec})
	if err != nil {
		t.Fatal(err)
	}
	path := DarwinLaunchAgentPath(home)
	rel, err := filepath.Rel(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(rel, "..") {
		t.Fatalf("plist %s is outside temp HOME %s", path, home)
	}

	on, err := tog.Enabled()
	if err != nil || on {
		t.Fatalf("enabled=%v err=%v", on, err)
	}
	if err := tog.Set(true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "<key>RunAtLoad</key>") || !strings.Contains(text, "<true/>") {
		t.Fatalf("plist missing RunAtLoad:\n%s", text)
	}
	if !strings.Contains(text, "<string>"+exec+"</string>") || !strings.Contains(text, LaunchAgentLabel) {
		t.Fatalf("plist contents:\n%s", text)
	}
	on, err = tog.Enabled()
	if err != nil || !on {
		t.Fatalf("after write enabled=%v err=%v", on, err)
	}

	if err := tog.Set(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("plist still present: %v", err)
	}
	on, err = tog.Enabled()
	if err != nil || on {
		t.Fatalf("after remove enabled=%v err=%v", on, err)
	}
}

func ciRunValueName(t *testing.T) string {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	// Distinct from the production Run value (AppName) so CI cannot enable the real app.
	return fmt.Sprintf("dnscrypt-proxy-updater-ci-%d-%x", os.Getpid(), b)
}
