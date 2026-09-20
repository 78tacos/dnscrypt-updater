package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntervalMinimum(t *testing.T) {
	t.Parallel()
	f := File{CheckInterval: "1s"}
	d, err := f.Interval()
	if err != nil {
		t.Fatal(err)
	}
	if d != MinInterval {
		t.Fatalf("got %s want %s", d, MinInterval)
	}
	f.CheckInterval = "12h"
	d, err = f.Interval()
	if err != nil {
		t.Fatal(err)
	}
	if d != 12*time.Hour {
		t.Fatalf("got %s", d)
	}
	f.CheckInterval = "nope"
	if _, err := f.Interval(); err == nil {
		t.Fatal("want parse error")
	}
}

func TestSkipAndSnooze(t *testing.T) {
	t.Parallel()
	f := File{SkipVersion: "v2.1.18"}
	if !f.Skips("2.1.18") {
		t.Fatal("expected skip")
	}
	if f.Skips("2.1.19") {
		t.Fatal("newer tag should not be skipped")
	}
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	f.SnoozeUntil = now.Add(time.Hour).Format(time.RFC3339)
	if !f.Snoozed(now) {
		t.Fatal("should be snoozed")
	}
	if f.Snoozed(now.Add(2 * time.Hour)) {
		t.Fatal("snooze expired")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultFile()
	cfg.CurrentVersion = "2.1.14"
	cfg.BinaryPath = `/opt/dnscrypt-proxy/dnscrypt-proxy`
	if err := SaveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentVersion != "2.1.14" || got.BinaryPath != cfg.BinaryPath || !got.Notify {
		t.Fatalf("%+v", got)
	}
	missing, err := LoadFile(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if missing.CheckInterval != DefaultInterval.String() {
		t.Fatalf("default %+v", missing)
	}
	if !missing.SetSystemDNS || !missing.ManageService {
		t.Fatalf("install defaults %+v", missing)
	}
}

func TestLoadFileKeepsInstallDefaultsWhenOmitted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"check_interval":"12h","notify":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SetSystemDNS || !got.ManageService {
		t.Fatalf("omitted fields should keep defaults %+v", got)
	}
}

func TestResolvePathsExplicit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "custom.json")
	p, err := ResolvePaths(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p.File != cfg {
		t.Fatalf("file %s", p.File)
	}
	if p.State != filepath.Join(dir, "state.json") {
		t.Fatalf("state %s", p.State)
	}
	if p.Pending != filepath.Join(dir, "pending") {
		t.Fatalf("pending %s", p.Pending)
	}
}

func TestSaveStateAtomic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st := State{CachedTag: "2.1.18", ETag: `"x"`}
	if err := SaveState(path, st); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CachedTag != "2.1.18" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp leftover: %v", err)
	}
}
