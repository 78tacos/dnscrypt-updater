package proxyconf

import (
	"os"
	"strings"
	"testing"
)

func TestPQDNSCryptEnable(t *testing.T) {
	t.Parallel()
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f, ok := cat.FieldByPath("pqdnscrypt")
	if !ok {
		t.Fatal("pqdnscrypt missing from catalog")
	}
	t.Logf("type=%s default_raw=%q commented=%v default=%v", f.Type, f.DefaultRaw, f.Commented, f.Default)
	raw, err := os.ReadFile("upstream/example-dnscrypt-proxy.toml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ApplyPatches(string(raw), []Patch{{Path: "pqdnscrypt", Enabled: true, Value: mustJSON(true)}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "pqdnscrypt") {
			found = true
			t.Log("line:", trim)
			if trim != "pqdnscrypt = true" {
				t.Fatalf("want active pqdnscrypt = true, got %q", trim)
			}
		}
		if strings.HasPrefix(trim, "# pqdnscrypt") {
			t.Fatalf("still commented: %q", trim)
		}
	}
	if !found {
		t.Fatal("pqdnscrypt line missing after patch")
	}

	// UI may omit value when only the enable checkbox is flipped
	out2, err := ApplyPatches(string(raw), []Patch{{Path: "pqdnscrypt", Enabled: true}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out2, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "pqdnscrypt") {
			t.Log("no-value line:", trim)
			if !strings.HasPrefix(trim, "pqdnscrypt =") || strings.HasPrefix(trim, "#") {
				t.Fatalf("bad: %q", trim)
			}
		}
	}

	cur := CurrentValues(string(raw), cat)
	v := cur["pqdnscrypt"]
	t.Logf("current present=%v value=%v raw=%q comment=%v", v.Present, v.Decoded, v.Raw, v.Comment)
	if v.Present {
		t.Fatal("example pqdnscrypt should start commented")
	}
}

func TestPQDNSCryptInsertWhenAbsent(t *testing.T) {
	t.Parallel()
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	src := "listen_addresses = ['127.0.0.1:53']\ncache = true\n\n[query_log]\nfile = 'query.log'\n"
	out, err := ApplyPatches(src, []Patch{{Path: "pqdnscrypt", Enabled: true, Value: mustJSON(true)}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "pqdnscrypt = true") {
		t.Fatalf("missing insert:\n%s", out)
	}
	if strings.Contains(out, "# pqdnscrypt") {
		t.Fatal("commented")
	}
	// must stay in root section, before [query_log]
	idxPQ := strings.Index(out, "pqdnscrypt = true")
	idxQL := strings.Index(out, "[query_log]")
	if idxPQ < 0 || idxQL < 0 || idxPQ > idxQL {
		t.Fatalf("bad placement pq=%d ql=%d\n%s", idxPQ, idxQL, out)
	}
}
