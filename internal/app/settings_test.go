package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/apply"
	"github.com/78tacos/dnscrypt-updater/internal/check"
	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/notify"
	"github.com/78tacos/dnscrypt-updater/internal/proxyconf"
)

func TestProxyPaths(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		Opts: Options{InstallDir: filepath.Join("C:", "opt-proxy")},
		cfg:  config.File{ManageService: true},
		last: check.Result{},
	}
	dir, bin := rt.proxyPaths()
	if dir != rt.Opts.InstallDir {
		t.Fatalf("dir %q", dir)
	}
	wantBin := filepath.Join(dir, apply.ProxyBinaryName(runtime.GOOS))
	if bin != wantBin {
		t.Fatalf("bin %q want %q", bin, wantBin)
	}
}

func TestApplyPendingMissing(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		Paths: config.Paths{Pending: filepath.Join(t.TempDir(), "pending")},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := rt.ApplyPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no pending settings") {
		t.Fatalf("got %v", err)
	}
}

func TestApplyPendingWritesInstallDir(t *testing.T) {
	t.Parallel()
	orig := notify.Send
	t.Cleanup(func() { notify.Send = orig })
	notify.Send = func(string, string, string) error { return nil }
	install := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("cache = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := proxyconf.SavePending(staging, pending, install, "needs admin"); err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{
		Opts:  Options{InstallDir: install, BinaryPath: filepath.Join(install, apply.ProxyBinaryName(runtime.GOOS)), NoService: true},
		Paths: config.Paths{Pending: pending},
		cfg:   config.File{ManageService: false},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Applier: &apply.Applier{
			Elevated: func() bool { return true },
			RunCheck: func(context.Context, string, string) error { return nil },
		},
	}
	res, err := rt.ApplyPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied {
		t.Fatalf("%+v", res)
	}
	got, err := os.ReadFile(filepath.Join(install, "dnscrypt-proxy.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cache = true\n" {
		t.Fatalf("%q", got)
	}
	if proxyconf.ReadPending(pending).Present {
		t.Fatal("pending not cleared")
	}
}
