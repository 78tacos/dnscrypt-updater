package apply

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestWindowsCommandLineQuotesArgs(t *testing.T) {
	t.Parallel()
	got := windowsCommandLine([]string{"-install", "-quiet", "-config", `C:\Users\A B\config.json`})
	want := "-install -quiet -config \"C:\\Users\\A B\\config.json\""
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if windowsCommandLine(nil) != "" {
		t.Fatal("empty args")
	}
	if escapeWindowsArg("") != `""` {
		t.Fatal("empty arg")
	}
	if got := escapeWindowsArg(`C:\Program Files\`); got != `"C:\Program Files\\"` {
		t.Fatalf("trailing slash: %q", got)
	}
	if got := escapeWindowsArg(`say "hi"`); got != `"say \"hi\""` {
		t.Fatalf("embedded quote: %q", got)
	}
}

func TestLaunchElevatedRunAsAndDecline(t *testing.T) {
	t.Parallel()
	var verb, file, params string
	code, err := launchElevated(`C:\Program Files\dnscrypt-proxy-updater.exe`, []string{
		"-install", "-quiet", "-config", `C:\Users\A B\config.json`,
	}, func(v, f, p string) (int, error) {
		verb, file, params = v, f, p
		return 0, syscall.Errno(1223)
	})
	if code != 1223 || !errors.Is(err, ErrElevationCancelled) {
		t.Fatalf("code %d err %v", code, err)
	}
	if verb != "runas" {
		t.Fatalf("verb %q", verb)
	}
	if file != `C:\Program Files\dnscrypt-proxy-updater.exe` {
		t.Fatalf("file %q", file)
	}
	for _, want := range []string{"-install", "-quiet", `"C:\Users\A B\config.json"`} {
		if !strings.Contains(params, want) {
			t.Fatalf("params %q missing %q", params, want)
		}
	}
}

func TestLaunchElevatedWaitsForExitCode(t *testing.T) {
	t.Parallel()
	code, err := launchElevated(`C:\updater.exe`, []string{"-install"}, func(verb, file, params string) (int, error) {
		if verb != "runas" || file != `C:\updater.exe` || params != "-install" {
			t.Fatalf("launch %s %s %s", verb, file, params)
		}
		return 0, nil
	})
	if err != nil || code != 0 {
		t.Fatalf("code %d err %v", code, err)
	}

	code, err = launchElevated(`C:\updater.exe`, []string{"-install"}, func(string, string, string) (int, error) {
		return 1, errors.New("shell failed")
	})
	if code != 1 || err == nil || errors.Is(err, ErrElevationCancelled) {
		t.Fatalf("code %d err %v", code, err)
	}
}

func TestLaunchElevatedRequiresRunner(t *testing.T) {
	t.Parallel()
	_, err := launchElevated(`C:\updater.exe`, []string{"-install"}, nil)
	if !errors.Is(err, ErrElevationUnsupported) {
		t.Fatalf("err %v", err)
	}
}
