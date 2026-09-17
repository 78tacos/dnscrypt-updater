package notify

import (
	"strings"
	"testing"
)

func TestUpdateAvailableMessage(t *testing.T) {
	t.Parallel()
	var title, msg, icon string
	orig := Send
	t.Cleanup(func() { Send = orig })
	Send = func(ti, m, i string) error {
		title, msg, icon = ti, m, i
		return nil
	}
	if err := UpdateAvailable("2.1.14", "2.1.18", "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18"); err != nil {
		t.Fatal(err)
	}
	if title != "dnscrypt-proxy update available" {
		t.Fatalf("title %q", title)
	}
	if icon != "" {
		t.Fatalf("icon %q", icon)
	}
	for _, want := range []string{"2.1.14 → 2.1.18", "-install", "github.com/DNSCrypt/dnscrypt-proxy"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q: %q", want, msg)
		}
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	orig := Send
	t.Cleanup(func() { Send = orig })
	Send = func(string, string, string) error { return nil }
	if err := NotFound(); err != nil {
		t.Fatal(err)
	}
}
