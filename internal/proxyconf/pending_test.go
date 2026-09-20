package proxyconf

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPrivilegeError(t *testing.T) {
	t.Parallel()
	if IsPrivilegeError(nil) {
		t.Fatal("nil")
	}
	if !IsPrivilegeError(os.ErrPermission) {
		t.Fatal("ErrPermission")
	}
	pe := &os.PathError{Op: "open", Path: "x", Err: os.ErrPermission}
	if !IsPrivilegeError(pe) {
		t.Fatal("PathError")
	}
	if !IsPrivilegeError(fmtErr("dnscrypt-proxy-updater: administrator permission was declined")) {
		t.Fatal("declined")
	}
	if !IsPrivilegeError(fmtErr(`write dnscrypt-proxy.toml: open C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.toml: The process cannot access the file because it is being used by another process.`)) {
		t.Fatal("sharing violation")
	}
	if !IsPrivilegeError(fmtErr("administrator apply failed: service start failed: exit status 1")) {
		t.Fatal("admin apply failed")
	}
	if IsPrivilegeError(fmtErr("dnscrypt-proxy -check: bad key")) {
		t.Fatal("check errors are not privilege errors")
	}
}

func fmtErr(s string) error { return &plainError{s} }

type plainError struct{ s string }

func (e *plainError) Error() string { return e.s }

func TestSaveReadClearPending(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "public-resolvers.md"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := SavePending(staging, pending, "/opt/dnscrypt-proxy", "needs admin")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Present || st.InstallDir != "/opt/dnscrypt-proxy" {
		t.Fatalf("%+v", st)
	}
	if strings.Contains(strings.Join(st.Files, ","), "public-resolvers.md") {
		t.Fatalf("queued cache: %v", st.Files)
	}
	got := ReadPending(pending)
	if !got.Present || got.Reason != "needs admin" {
		t.Fatalf("%+v", got)
	}
	var buf bytes.Buffer
	if err := WritePendingZip(pending, &buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "dnscrypt-proxy.toml" {
		t.Fatalf("%v", zr.File)
	}
	if err := ClearPending(pending); err != nil {
		t.Fatal(err)
	}
	if ReadPending(pending).Present {
		t.Fatal("still present")
	}
}

func TestTryCommitPrivilegeQueuesPending(t *testing.T) {
	t.Parallel()
	install := t.TempDir()
	staging := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("cache = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checked := false
	elevated := false
	res, err := TryCommit(context.Background(), ApplyEnv{
		InstallDir: install,
		BinaryPath: "dnscrypt-proxy",
		Check: func(context.Context, string, string) error {
			checked = true
			return nil
		},
		WriteErr: os.ErrPermission,
	}, staging, pending, false, func(context.Context, string) error {
		elevated = true
		return os.ErrPermission
	})
	if err != nil {
		t.Fatal(err)
	}
	if !checked || !elevated {
		t.Fatalf("checked=%v elevated=%v", checked, elevated)
	}
	if !res.Pending || res.Applied || res.PendingDir != pending {
		t.Fatalf("%+v", res)
	}
	if !ReadPending(pending).Present {
		t.Fatal("expected queued files")
	}
	got, _ := os.ReadFile(filepath.Join(install, "dnscrypt-proxy.toml"))
	if string(got) != "cache = false\n" {
		t.Fatalf("live file should be unchanged: %q", got)
	}
}

func TestTryCommitElevateExitStatusQueuesPending(t *testing.T) {
	t.Parallel()
	install := t.TempDir()
	staging := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("cache = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Bare exec.ExitError text is not IsPrivilegeError; admin apply still
	// must fall back to AppData instead of "Save failed: exit status 1".
	res, err := TryCommit(context.Background(), ApplyEnv{
		InstallDir: install,
		WriteErr:   os.ErrPermission,
	}, staging, pending, false, func(context.Context, string) error {
		return fmtErr("exit status 1")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pending || res.Applied {
		t.Fatalf("%+v", res)
	}
	got := ReadPending(pending)
	if !got.Present || !strings.Contains(got.Reason, "exit status 1") {
		t.Fatalf("%+v", got)
	}
}

func TestTryCommitIgnoresWritableProbeWhenCommitWorks(t *testing.T) {
	t.Parallel()
	install := t.TempDir()
	staging := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("cache = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	elevated := false
	res, err := TryCommit(context.Background(), ApplyEnv{InstallDir: install}, staging, pending, false, func(context.Context, string) error {
		elevated = true
		return os.ErrPermission
	})
	if err != nil {
		t.Fatal(err)
	}
	if elevated {
		t.Fatal("must not elevate when in-place commit works")
	}
	if !res.Applied || res.Pending {
		t.Fatalf("%+v", res)
	}
}

func TestTryCommitCheckErrorDoesNotQueue(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("bad = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	elevated := false
	_, err := TryCommit(context.Background(), ApplyEnv{
		InstallDir: t.TempDir(),
		BinaryPath: "dnscrypt-proxy",
		Check:      func(context.Context, string, string) error { return fmtErr("dnscrypt-proxy -check: bad key") },
	}, staging, pending, false, func(context.Context, string) error {
		elevated = true
		return os.ErrPermission
	})
	if err == nil {
		t.Fatal("want check error")
	}
	if elevated {
		t.Fatal("must not elevate or queue invalid config")
	}
	if ReadPending(pending).Present {
		t.Fatal("queued invalid config")
	}
}

func TestTryCommitWritableSuccessClearsPending(t *testing.T) {
	t.Parallel()
	install := t.TempDir()
	staging := t.TempDir()
	pending := filepath.Join(t.TempDir(), "pending")
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("cache = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("cache = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SavePending(staging, pending, install, "stale"); err != nil {
		t.Fatal(err)
	}
	res, err := TryCommit(context.Background(), ApplyEnv{InstallDir: install}, staging, pending, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Pending {
		t.Fatalf("%+v", res)
	}
	got, err := os.ReadFile(filepath.Join(install, "dnscrypt-proxy.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cache = true\n" {
		t.Fatalf("%q", got)
	}
	if ReadPending(pending).Present {
		t.Fatal("stale pending should be cleared")
	}
}
