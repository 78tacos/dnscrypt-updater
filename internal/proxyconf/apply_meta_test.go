package proxyconf

import (
	"path/filepath"
	"testing"
)

func TestApplyMetaRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteApplyMeta(dir, ApplyMeta{
		InstallDir: `C:\Program Files\dnscrypt-proxy`,
		BinaryPath: `C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe`,
	}); err != nil {
		t.Fatal(err)
	}
	got := ReadApplyMeta(dir)
	if got.InstallDir != `C:\Program Files\dnscrypt-proxy` {
		t.Fatalf("install: %#v", got)
	}
	if got.BinaryPath != `C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe` {
		t.Fatalf("bin: %#v", got)
	}
	if ReadApplyMeta(filepath.Join(dir, "missing")).InstallDir != "" {
		t.Fatal("expected empty meta")
	}
}
