package app

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/apply"
	"github.com/78tacos/dnscrypt-updater/internal/check"
	"github.com/78tacos/dnscrypt-updater/internal/config"
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
