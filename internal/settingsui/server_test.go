package settingsui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
