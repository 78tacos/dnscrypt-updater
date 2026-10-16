package proxyconf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyErrorRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ClearApplyError(dir)
	if got := ReadApplyError(dir); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	WriteApplyError(dir, fmtErr("service start failed: exit status 1"))
	if got := ReadApplyError(dir); got != "service start failed: exit status 1" {
		t.Fatalf("%q", got)
	}
	ClearApplyError(dir)
	if _, err := os.Stat(filepath.Join(dir, applyErrorName)); !os.IsNotExist(err) {
		t.Fatalf("expected removed: %v", err)
	}
}

func TestWriteApplyErrorIgnoresEmpty(t *testing.T) {
	t.Parallel()
	WriteApplyError("", fmtErr("x"))
	WriteApplyError(t.TempDir(), nil)
}
