package app

import (
	"context"
	"fmt"
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

func TestApplyPendingViaWindowsElevation(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("pqdnscrypt = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := proxyconf.SavePending(staging, pending, install, "access denied"); err != nil {
		t.Fatal(err)
	}

	relaunched := false
	rt := &Runtime{
		Opts:  Options{InstallDir: install, BinaryPath: filepath.Join(install, "dnscrypt-proxy.exe"), NoService: true},
		Paths: config.Paths{Pending: pending, File: filepath.Join(t.TempDir(), "config.json")},
		cfg:   config.File{ManageService: false, InstallDir: install},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Applier: &apply.Applier{
			GOOS:     "windows",
			Elevated: func() bool { return false },
			RunCheck: func(context.Context, string, string) error { return nil },
			Relaunch: func(args []string) (int, error) {
				relaunched = true
				stagingArg := flagValue(args, "-staging")
				if stagingArg == "" {
					t.Fatal("expected -staging")
				}
				meta := proxyconf.ReadApplyMeta(stagingArg)
				if meta.InstallDir != install {
					t.Fatalf("meta install %q want %q", meta.InstallDir, install)
				}
				// Simulate elevated -apply-config committing the pending bundle.
				src := filepath.Join(stagingArg, "dnscrypt-proxy.toml")
				dst := filepath.Join(install, "dnscrypt-proxy.toml")
				b, err := os.ReadFile(src)
				if err != nil {
					return 1, err
				}
				if err := os.WriteFile(dst, b, 0o644); err != nil {
					return 1, err
				}
				return 0, nil
			},
		},
	}
	res, err := rt.ApplyPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !relaunched || !res.Applied {
		t.Fatalf("relaunched=%v res=%+v", relaunched, res)
	}
	got, _ := os.ReadFile(filepath.Join(install, "dnscrypt-proxy.toml"))
	if string(got) != "pqdnscrypt = true\n" {
		t.Fatalf("%q", got)
	}
	if proxyconf.ReadPending(pending).Present {
		t.Fatal("pending should be cleared after elevated apply")
	}
}

func TestTryCommitElevateFailThenApplyPending(t *testing.T) {
	t.Parallel()
	orig := notify.Send
	t.Cleanup(func() { notify.Send = orig })
	notify.Send = func(string, string, string) error { return nil }

	install := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("cache = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := proxyconf.TryCommit(context.Background(), proxyconf.ApplyEnv{
		InstallDir: install,
		WriteErr:   os.ErrPermission,
	}, staging, pending, false, func(context.Context, string) error {
		return fmt.Errorf("administrator apply failed: exit status 1")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pending || !proxyconf.ReadPending(pending).Present {
		t.Fatalf("%+v", res)
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
	applied, err := rt.ApplyPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied {
		t.Fatalf("%+v", applied)
	}
	got, _ := os.ReadFile(filepath.Join(install, "dnscrypt-proxy.toml"))
	if string(got) != "cache = true\n" {
		t.Fatalf("after apply pending: %q", got)
	}
	if proxyconf.ReadPending(pending).Present {
		t.Fatal("pending still present")
	}
}

func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
