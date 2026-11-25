package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLabels(t *testing.T) {
	t.Parallel()
	if Label("windows") != "Start with Windows" {
		t.Fatal(Label("windows"))
	}
	if Label("linux") != "Start with Linux" {
		t.Fatal(Label("linux"))
	}
	if Label("darwin") != "Start at login" {
		t.Fatal(Label("darwin"))
	}
	if !strings.Contains(Help("windows"), `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`) {
		t.Fatal(Help("windows"))
	}
	if !strings.Contains(Help("linux"), "XDG_CONFIG_HOME") {
		t.Fatal(Help("linux"))
	}
	if !strings.Contains(Help("darwin"), "RunAtLoad") {
		t.Fatal(Help("darwin"))
	}
}

func TestLinuxAutostartEnableDisable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg")
	exec := "/usr/local/bin/dnscrypt-proxy-updater"
	tog, err := ForOS("linux", Config{Home: home, XDGConfigHome: xdg, ExecPath: exec})
	if err != nil {
		t.Fatal(err)
	}
	if tog.Label() != "Start with Linux" {
		t.Fatal(tog.Label())
	}
	on, err := tog.Enabled()
	if err != nil || on {
		t.Fatalf("enabled=%v err=%v", on, err)
	}
	if err := tog.Set(false); err != nil {
		t.Fatal(err)
	}
	if err := tog.Set(true); err != nil {
		t.Fatal(err)
	}
	on, err = tog.Enabled()
	if err != nil || !on {
		t.Fatalf("after enable enabled=%v err=%v", on, err)
	}
	path := LinuxAutostartPath(home, xdg, AppName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "Exec="+exec) || !strings.Contains(text, "X-GNOME-Autostart-enabled=true") {
		t.Fatalf("desktop file:\n%s", text)
	}
	homeFile := filepath.Join(home, ".config", "autostart", AppName+".desktop")
	if _, err := os.Stat(homeFile); !os.IsNotExist(err) {
		t.Fatalf("XDG_CONFIG_HOME ignored, stat %s err=%v", homeFile, err)
	}
	if err := tog.Set(false); err != nil {
		t.Fatal(err)
	}
	on, err = tog.Enabled()
	if err != nil || on {
		t.Fatalf("after disable enabled=%v err=%v", on, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("desktop file still present: %v", err)
	}
}

func TestLinuxAutostartHomeAndQuoting(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	exec := "/home/someone/My Apps/dnscrypt-proxy-updater"
	tog, err := ForOS("linux", Config{Home: home, ExecPath: exec})
	if err != nil {
		t.Fatal(err)
	}
	if err := tog.Set(true); err != nil {
		t.Fatal(err)
	}
	path := LinuxAutostartPath(home, "", AppName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `Exec="/home/someone/My Apps/dnscrypt-proxy-updater"`) {
		t.Fatalf("quoting:\n%s", b)
	}
	if err := os.WriteFile(path, []byte("[Desktop Entry]\nHidden=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	on, err := tog.Enabled()
	if err != nil || on {
		t.Fatalf("hidden should be off: %v %v", on, err)
	}
}

func TestLinuxRelativeXDGConfigHome(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	got := LinuxAutostartPath(home, "config", AppName)
	want := filepath.Join(home, ".config", "autostart", AppName+".desktop")
	if got != want {
		t.Fatalf("relative XDG_CONFIG_HOME: got %s want %s", got, want)
	}
	abs := filepath.Join(t.TempDir(), "xdg")
	got = LinuxAutostartPath(home, abs, AppName)
	want = filepath.Join(abs, "autostart", AppName+".desktop")
	if got != want {
		t.Fatalf("absolute XDG_CONFIG_HOME: got %s want %s", got, want)
	}
}

func TestDesktopExec(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"/usr/local/bin/dnscrypt-proxy-updater", "/usr/local/bin/dnscrypt-proxy-updater"},
		{"/home/someone/My Apps/dnscrypt-proxy-updater", `"/home/someone/My Apps/dnscrypt-proxy-updater"`},
		{"/home/alice/apps%z/updater", "/home/alice/apps%%z/updater"},
		{"/home/o'brien/app", `"/home/o'brien/app"`},
		{"/opt/app$dir/updater", `"/opt/app\$dir/updater"`},
		{"/opt/app`id`/updater", "\"/opt/app\\`id\\`/updater\""},
		{"/opt/app\\bin\\updater", `"/opt/app\\bin\\updater"`},
	}
	for _, tc := range cases {
		if got := desktopExec(tc.in); got != tc.want {
			t.Fatalf("desktopExec(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestDarwinLaunchAgentEnableDisable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	exec := "/Applications/dnscrypt-proxy-updater"
	tog, err := ForOS("darwin", Config{Home: home, ExecPath: exec})
	if err != nil {
		t.Fatal(err)
	}
	if tog.Label() != "Start at login" {
		t.Fatal(tog.Label())
	}
	on, err := tog.Enabled()
	if err != nil || on {
		t.Fatalf("enabled=%v err=%v", on, err)
	}
	if err := tog.Set(true); err != nil {
		t.Fatal(err)
	}
	on, err = tog.Enabled()
	if err != nil || !on {
		t.Fatalf("after enable enabled=%v err=%v", on, err)
	}
	path := DarwinLaunchAgentPath(home)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "<key>RunAtLoad</key>") || !strings.Contains(text, "<true/>") {
		t.Fatalf("plist missing RunAtLoad:\n%s", text)
	}
	if !strings.Contains(text, "<string>"+exec+"</string>") {
		t.Fatalf("plist exec:\n%s", text)
	}
	if !strings.Contains(text, LaunchAgentLabel) {
		t.Fatal("missing label")
	}
	spaced, err := ForOS("darwin", Config{Home: home, ExecPath: `/Users/a/My Apps/updater & co`})
	if err != nil {
		t.Fatal(err)
	}
	if err := spaced.Set(true); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "My Apps/updater &amp; co") {
		t.Fatalf("xml escape:\n%s", b)
	}
	on, err = spaced.Enabled()
	if err != nil || !on {
		t.Fatalf("escaped plist enabled=%v err=%v", on, err)
	}
	if err := spaced.Set(false); err != nil {
		t.Fatal(err)
	}
	on, err = spaced.Enabled()
	if err != nil || on {
		t.Fatalf("after disable enabled=%v err=%v", on, err)
	}
	off := strings.ReplaceAll(string(b), "<true/>", "<false/>")
	if err := os.WriteFile(path, []byte(off), 0o644); err != nil {
		t.Fatal(err)
	}
	on, err = tog.Enabled()
	if err != nil || on {
		t.Fatalf("RunAtLoad false should be off: %v %v", on, err)
	}
}

func TestWindowsRunKeyFake(t *testing.T) {
	t.Parallel()
	reg := newMemRegistry()
	exec := `C:\Program Files\dnscrypt-proxy-updater.exe`
	tog, err := ForOS("windows", Config{ExecPath: exec, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	if tog.Label() != "Start with Windows" {
		t.Fatal(tog.Label())
	}
	on, err := tog.Enabled()
	if err != nil || on {
		t.Fatalf("enabled=%v err=%v", on, err)
	}
	if err := tog.Set(false); err != nil {
		t.Fatal(err)
	}
	if err := tog.Set(true); err != nil {
		t.Fatal(err)
	}
	on, err = tog.Enabled()
	if err != nil || !on {
		t.Fatalf("after enable enabled=%v err=%v", on, err)
	}
	got, ok, err := reg.Get(AppName)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	if got != `"`+exec+`"` {
		t.Fatalf("run value %q", got)
	}
	moved := `C:\Tools\dnscrypt-proxy-updater.exe`
	again, err := ForOS("windows", Config{ExecPath: moved, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Set(true); err != nil {
		t.Fatal(err)
	}
	got, _, _ = reg.Get(AppName)
	if got != moved {
		t.Fatalf("unquoted update %q", got)
	}
	if err := again.Set(false); err != nil {
		t.Fatal(err)
	}
	on, err = again.Enabled()
	if err != nil || on {
		t.Fatalf("after disable enabled=%v err=%v", on, err)
	}
	if _, ok, _ := reg.Get(AppName); ok {
		t.Fatal("value remains")
	}
	if _, err := ForOS("windows", Config{ExecPath: exec}); err == nil {
		t.Fatal("expected registry requirement")
	}
}

func TestFormatRunCommand(t *testing.T) {
	t.Parallel()
	if got := FormatRunCommand(`C:\app.exe`); got != `C:\app.exe` {
		t.Fatal(got)
	}
	if got := FormatRunCommand(`C:\Program Files\app.exe`); got != `"C:\Program Files\app.exe"` {
		t.Fatal(got)
	}
	if got := FormatRunCommand(`"already quoted"`); got != `"already quoted"` {
		t.Fatal(got)
	}
}

type memRegistry struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemRegistry() *memRegistry {
	return &memRegistry{m: map[string]string{}}
}

func (r *memRegistry) Get(name string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[name]
	return v, ok, nil
}

func (r *memRegistry) Set(name, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = value
	return nil
}

func (r *memRegistry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, name)
	return nil
}
