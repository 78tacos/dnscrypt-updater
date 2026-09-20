package settingsui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/proxyconf"
)

func TestStateAndApplyToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	toml := filepath.Join(dir, "dnscrypt-proxy.toml")
	if err := os.WriteFile(toml, []byte("listen_addresses = ['127.0.0.1:53']\nunknown_future_key = 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "dnscrypt-proxy")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{
		Locate: func() (string, string, bool) { return dir, bin, false },
		Check:  func(ctx context.Context, b, c string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/state?token="+srv.token, nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("state %d", res.StatusCode)
	}
	var st snapshot
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if !st.TomlExists || st.Current["listen_addresses"].Present == false {
		t.Fatalf("%+v", st.Current["listen_addresses"])
	}
	if _, ok := st.Catalog.FieldByPath("monitoring_ui.enabled"); !ok {
		t.Fatal("catalog missing monitoring_ui.enabled")
	}

	body := `{"patches":[{"path":"force_tcp","enabled":true,"value":true}]}`
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/apply?token="+srv.token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("apply %d %s", res.StatusCode, b)
	}
	got, err := os.ReadFile(toml)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "unknown_future_key = 9") {
		t.Fatalf("lost unknown:\n%s", s)
	}
	if !strings.Contains(s, "force_tcp = true") {
		t.Fatalf("missing patch:\n%s", s)
	}

	for _, path := range []string{"/app.js", "/style.css"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || len(b) < 50 {
			t.Fatalf("%s %d len=%d", path, res.StatusCode, len(b))
		}
	}
}

func TestIndexHTML(t *testing.T) {
	t.Parallel()
	srv, err := New(Options{Locate: func() (string, string, bool) { return "", "", false }})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(b), "dnscrypt-proxy settings") {
		t.Fatalf("%s", b[:min(200, len(b))])
	}
}

func TestApplyPresetKeepsComments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "proxyconf", "upstream", "example-dnscrypt-proxy.toml"))
	if err != nil {
		t.Fatal(err)
	}
	toml := filepath.Join(dir, "dnscrypt-proxy.toml")
	if err := os.WriteFile(toml, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "dnscrypt-proxy")
	if err := os.WriteFile(bin, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{
		Locate: func() (string, string, bool) { return dir, bin, false },
		Check:  func(context.Context, string, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	body := `{"preset":"tor"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/apply?token="+srv.token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("apply %d %s", res.StatusCode, b)
	}
	got, err := os.ReadFile(toml)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "force_tcp = true") {
		t.Fatalf("tor preset missing force_tcp")
	}
	if !strings.Contains(s, "socks5://dnscrypt:dnscrypt@127.0.0.1:9050") {
		t.Fatal("tor preset missing proxy")
	}
	if !strings.Contains(s, "Online documentation is available here") {
		t.Fatal("lost header comment")
	}
}

func TestApplyQueuesPendingOnElevationDecline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	toml := filepath.Join(dir, "dnscrypt-proxy.toml")
	if err := os.WriteFile(toml, []byte("listen_addresses = ['127.0.0.1:53']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "dnscrypt-proxy")
	if err := os.WriteFile(bin, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(t.TempDir(), "pending")
	var pendingCalls int
	srv, err := New(Options{
		Locate:         func() (string, string, bool) { return dir, bin, false },
		Check:          func(context.Context, string, string) error { return nil },
		CanWrite:       func(string) bool { return false },
		NeedsElevation: func() bool { return true },
		Elevate:        func(context.Context, string) error { return os.ErrPermission },
		PendingDir:     func() string { return pending },
		OnPending:      func(proxyconf.PendingStatus) { pendingCalls++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	body := `{"patches":[{"path":"force_tcp","enabled":true,"value":true}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/apply?token="+srv.token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("apply %d %s", res.StatusCode, b)
	}
	var out struct {
		Pending    bool   `json:"pending"`
		Applied    bool   `json:"applied"`
		PendingDir string `json:"pending_dir"`
		Message    string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Pending || out.Applied || out.PendingDir != pending {
		t.Fatalf("%+v", out)
	}
	if pendingCalls != 1 {
		t.Fatalf("OnPending calls %d", pendingCalls)
	}
	live, err := os.ReadFile(toml)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(live), "force_tcp = true") {
		t.Fatal("live install should be unchanged")
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/pending.zip?token="+srv.token, nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("zip %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "dnscrypt-proxy.toml" {
		t.Fatalf("%v", zr.File)
	}

	stReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/state?token="+srv.token, nil)
	stRes, err := http.DefaultClient.Do(stReq)
	if err != nil {
		t.Fatal(err)
	}
	defer stRes.Body.Close()
	var st snapshot
	if err := json.NewDecoder(stRes.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if !st.Pending.Present || !st.NeedsElevation {
		t.Fatalf("state pending %+v elev=%v", st.Pending, st.NeedsElevation)
	}

	del, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/pending?token="+srv.token, nil)
	delRes, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	defer delRes.Body.Close()
	if delRes.StatusCode != 200 {
		t.Fatalf("delete %d", delRes.StatusCode)
	}
	if proxyconf.ReadPending(pending).Present {
		t.Fatal("pending still present")
	}
}
