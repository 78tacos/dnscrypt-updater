package proxyconf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuggestions(t *testing.T) {
	t.Parallel()
	cat := Catalog{Fields: []Field{
		{Path: "http3_probe", Type: TypeBool, Warning: true, Help: "WARNING: slow"},
	}}
	cur := map[string]Value{
		"ipv6_servers":                 {Path: "ipv6_servers", Present: true, Decoded: true},
		"http3_probe":                  {Path: "http3_probe", Present: true, Decoded: true},
		"monitoring_ui.enabled":        {Present: true, Decoded: true},
		"monitoring_ui.listen_address": {Present: true, Decoded: "0.0.0.0:8080", Raw: `"0.0.0.0:8080"`},
		"monitoring_ui.password":       {Present: true, Decoded: "changeme"},
		"server_names":                 {Present: true, Decoded: []string{"cloudflare"}},
	}
	got := Suggestions(cat, cur)
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	for _, id := range []string{"ipv6_servers", "http3_probe", "monitoring_bind", "monitoring_password", "server_names"} {
		if !ids[id] {
			t.Fatalf("missing %s in %#v", id, got)
		}
	}
}

func TestStageAndCommit(t *testing.T) {
	t.Parallel()
	install := t.TempDir()
	live := filepath.Join(install, "dnscrypt-proxy.toml")
	src := "listen_addresses = ['127.0.0.1:53']\nunknown_future_key = 1\n"
	if err := os.WriteFile(live, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := Catalog{Fields: []Field{
		{Path: "listen_addresses", Type: TypeStringList, Key: "listen_addresses"},
		{Path: "force_tcp", Type: TypeBool, Key: "force_tcp"},
	}}
	staging := t.TempDir()
	req := ApplyRequest{Patches: []Patch{
		{Path: "force_tcp", Enabled: true, Value: mustJSON(true)},
	}}
	if err := Stage(install, staging, req, cat); err != nil {
		t.Fatal(err)
	}
	checked := false
	started := false
	stopped := false
	env := ApplyEnv{
		InstallDir:    install,
		BinaryPath:    filepath.Join(install, "dnscrypt-proxy"),
		ManageService: true,
		Check: func(context.Context, string, string) error {
			checked = true
			return nil
		},
		StopService: func(context.Context, string) error {
			stopped = true
			return nil
		},
		StartService: func(context.Context, string) error {
			started = true
			return nil
		},
	}
	if _, err := Commit(context.Background(), env, staging); err != nil {
		t.Fatal(err)
	}
	if !checked || !started || !stopped {
		t.Fatalf("hooks check=%v stop=%v start=%v", checked, stopped, started)
	}
	got, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, "unknown_future_key = 1") {
		t.Fatalf("lost unknown:\n%s", body)
	}
	if !strings.Contains(body, "force_tcp = true") {
		t.Fatalf("missing patch:\n%s", body)
	}
}

func TestCommitSkipsResolverCaches(t *testing.T) {
	t.Parallel()
	install := t.TempDir()
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "dnscrypt-proxy.toml"), []byte("listen_addresses = ['127.0.0.1:53']\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "public-resolvers.md"), []byte("LIVE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "dnscrypt-proxy.toml"), []byte("listen_addresses = ['127.0.0.1:53']\nforce_tcp = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "public-resolvers.md"), []byte("STAGED"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Commit(context.Background(), ApplyEnv{
		InstallDir: install,
		Check:      func(context.Context, string, string) error { return nil },
	}, staging)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(install, "public-resolvers.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "LIVE" {
		t.Fatalf("resolver cache rewritten: %q", got)
	}
}

func TestRefuseUnknownListFile(t *testing.T) {
	t.Parallel()
	if _, err := resolveListPath("/tmp", "public-resolvers.md"); err == nil {
		t.Fatal("expected refuse")
	}
	if _, err := resolveListPath("/tmp", "../etc/passwd"); err == nil {
		t.Fatal("expected refuse")
	}
}
