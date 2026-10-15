package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/apply"
	"github.com/78tacos/dnscrypt-updater/internal/check"
	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/detect"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
	"github.com/78tacos/dnscrypt-updater/internal/notify"
	"github.com/78tacos/dnscrypt-updater/internal/version"
)

func TestInstallSetsSystemDNS(t *testing.T) {
	t.Parallel()
	on := true
	off := false
	cases := []struct {
		name  string
		cfg   config.File
		noDNS bool
		want  bool
	}{
		{name: "unset defaults on", cfg: config.File{}, want: true},
		{name: "explicit true", cfg: config.File{SetSystemDNS: &on}, want: true},
		{name: "explicit false", cfg: config.File{SetSystemDNS: &off}, want: false},
		{name: "cli opt-out", cfg: config.File{SetSystemDNS: &on}, noDNS: true, want: false},
		{name: "config false and cli opt-out", cfg: config.File{SetSystemDNS: &off}, noDNS: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := installSetsSystemDNS(tc.cfg, tc.noDNS); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestElevatedInstallArgs(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFile()
	args := elevatedInstallArgs(cfg, Options{}, `C:\Users\A B\config.json`)
	if !hasArg(args, "-install") || !hasArg(args, "-quiet") || !hasArg(args, `C:\Users\A B\config.json`) {
		t.Fatalf("args %q", args)
	}
	if hasArg(args, "-no-dns") || hasArg(args, "-no-service") {
		t.Fatalf("defaults should stay on: %q", args)
	}

	off := false
	cfg.SetSystemDNS = &off
	cfg.ManageService = false
	args = elevatedInstallArgs(cfg, Options{
		NoDNS:          true,
		InstallDir:     `C:\Program Files\dnscrypt-proxy`,
		BinaryPath:     `D:\proxy\dnscrypt-proxy.exe`,
		CurrentVersion: "2.1.14",
		NoNotify:       true,
	}, `C:\c.json`)
	for _, want := range []string{"-no-dns", "-no-service", "-install-dir", `C:\Program Files\dnscrypt-proxy`, "-binary-path", `D:\proxy\dnscrypt-proxy.exe`, "-current-version", "2.1.14", "-no-notify"} {
		if !hasArg(args, want) {
			t.Fatalf("missing %q in %q", want, args)
		}
	}
}

func TestTrayInstallPromptsForAdminAndReportsDecline(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFile()
	cfg.Notify = false
	var got []string
	rt := installRuntime(t, dir, cfg, &apply.Applier{
		GOOS:     "windows",
		Elevated: func() bool { return false },
		Relaunch: func(args []string) (int, error) {
			got = append([]string(nil), args...)
			return 1223, apply.ErrElevationCancelled
		},
	})
	orig := notify.Send
	t.Cleanup(func() { notify.Send = orig })
	var title, msg string
	notify.Send = func(ti, m, _ string) error {
		title, msg = ti, m
		return nil
	}

	// The tray Install item calls Runtime.Install.
	_, err := rt.Install(context.Background())
	if !errors.Is(err, apply.ErrElevationCancelled) {
		t.Fatalf("err %v", err)
	}
	if title != "dnscrypt-proxy install failed" || !strings.Contains(msg, "declined") {
		t.Fatalf("notification title %q msg %q", title, msg)
	}
	if !hasArg(got, "-install") || !hasArg(got, "-config") || !hasArg(got, rt.Paths.File) {
		t.Fatalf("relaunch args %q", got)
	}
	if hasArg(got, "-no-dns") {
		t.Fatalf("default-on DNS was opted out: %q", got)
	}
}

func TestTrayInstallSkipsUACWhenAlreadyElevated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.DefaultFile()
	cfg.Notify = false
	called := false
	rt := installRuntime(t, dir, cfg, &apply.Applier{
		GOOS:     "windows",
		Elevated: func() bool { return true },
		Relaunch: func([]string) (int, error) {
			called = true
			return 0, nil
		},
		MkdirTemp: func(string, string) (string, error) {
			return "", errors.New("stop-before-download")
		},
	})
	_, err := rt.Install(context.Background())
	if called {
		t.Fatal("already-elevated install relaunched")
	}
	if err == nil || !strings.Contains(err.Error(), "stop-before-download") {
		t.Fatalf("err %v", err)
	}
}

func TestTrayInstallWaitsForElevatedSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFile()
	cfg.Notify = false
	rt := installRuntime(t, dir, cfg, &apply.Applier{
		GOOS:     "windows",
		Elevated: func() bool { return false },
		Relaunch: func(args []string) (int, error) {
			if !hasArg(args, "-install") {
				return 1, errors.New("missing -install")
			}
			return 0, nil
		},
	})
	res, err := rt.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Elevated || res.Message == "" {
		t.Fatalf("%+v", res)
	}
}

func installRuntime(t *testing.T, dir string, cfg config.File, ap *apply.Applier) *Runtime {
	t.Helper()
	return &Runtime{
		Paths: config.Paths{
			Dir:   dir,
			File:  filepath.Join(dir, "config.json"),
			State: filepath.Join(dir, "state.json"),
			Log:   filepath.Join(dir, "x.log"),
		},
		Engine: check.Engine{
			GOOS:   "windows",
			GOARCH: "amd64",
			GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{
				TagName: "2.1.18",
				HTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
				Assets: []githubrel.Asset{
					{Name: "dnscrypt-proxy-win64-2.1.18.zip", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip"},
					{Name: "dnscrypt-proxy-win64-2.1.18.zip.minisig", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip.minisig"},
				},
			}}},
			Detect: detStub{res: detect.Result{Found: false, Source: detect.SourceNone}},
		},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:     cfg,
		Applier: ap,
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestCheckOnceJSONUpdateExit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	v, err := version.Parse("2.1.14")
	if err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{
		Opts: Options{CheckOnce: true, JSON: true},
		Paths: config.Paths{
			Dir:   dir,
			File:  filepath.Join(dir, "config.json"),
			State: filepath.Join(dir, "state.json"),
			Log:   filepath.Join(dir, "x.log"),
		},
		Engine: check.Engine{
			GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{
				TagName: "2.1.18",
				HTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
			}}},
			Detect: detStub{res: detect.Result{Found: true, Version: v, Source: detect.SourceBinary, Path: "/opt/dnscrypt-proxy/dnscrypt-proxy"}},
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg: config.File{Notify: true},
	}
	var buf bytes.Buffer
	code, err := rt.CheckOnce(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	var res check.Result
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatal(err, buf.String())
	}
	if !res.UpdateAvailable || res.RemoteVersion != "2.1.18" {
		t.Fatalf("%+v", res)
	}
}

type ghStub struct {
	res githubrel.Result
}

func (g *ghStub) Latest(context.Context, string) (githubrel.Result, error) { return g.res, nil }

type detStub struct {
	res detect.Result
}

func (d detStub) Detect(context.Context, string, string) detect.Result { return d.res }
