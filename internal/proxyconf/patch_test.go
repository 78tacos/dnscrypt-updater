package proxyconf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyPatchesPreserveUnknown(t *testing.T) {
	t.Parallel()
	src := `# keep this comment
listen_addresses = ['127.0.0.1:53']
# server_names = ['cloudflare']
unknown_future_key = 42

[monitoring_ui]
enabled = false
`
	cat := Catalog{Fields: []Field{
		{Path: "listen_addresses", Type: TypeStringList, Key: "listen_addresses"},
		{Path: "server_names", Type: TypeStringList, Key: "server_names"},
		{Path: "monitoring_ui.enabled", Type: TypeBool, Key: "enabled", Section: "monitoring_ui"},
	}}
	out, err := ApplyPatches(src, []Patch{
		{Path: "server_names", Enabled: true, Value: mustJSON([]string{"quad9"})},
		{Path: "monitoring_ui.enabled", Enabled: true, Value: mustJSON(true)},
	}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "unknown_future_key = 42") {
		t.Fatalf("lost unknown key:\n%s", out)
	}
	if !strings.Contains(out, "# keep this comment") {
		t.Fatalf("lost comment:\n%s", out)
	}
	if !strings.Contains(out, "server_names = ['quad9']") {
		t.Fatalf("did not uncomment/set server_names:\n%s", out)
	}
	if strings.Contains(out, "# server_names") {
		t.Fatalf("server_names still commented:\n%s", out)
	}
	if !strings.Contains(out, "enabled = true") {
		t.Fatalf("monitoring not enabled:\n%s", out)
	}
}

func TestApplyPatchesCommentOut(t *testing.T) {
	t.Parallel()
	src := "cache = true\n"
	cat := Catalog{Fields: []Field{{Path: "cache", Type: TypeBool, Key: "cache"}}}
	out, err := ApplyPatches(src, []Patch{{Path: "cache", Enabled: false, Value: mustJSON(true)}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# cache = true") {
		t.Fatalf("%s", out)
	}
}

func TestApplyPatchesInsertSection(t *testing.T) {
	t.Parallel()
	src := "listen_addresses = ['127.0.0.1:53']\n"
	cat := Catalog{Fields: []Field{
		{Path: "listen_addresses", Type: TypeStringList, Key: "listen_addresses"},
		{Path: "query_log.file", Type: TypeString, Key: "file", Section: "query_log"},
	}}
	out, err := ApplyPatches(src, []Patch{
		{Path: "query_log.file", Enabled: true, Value: mustJSON("query.log")},
	}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[query_log]") || !strings.Contains(out, "file = 'query.log'") {
		t.Fatalf("%s", out)
	}
}

func TestCurrentValues(t *testing.T) {
	t.Parallel()
	src := "listen_addresses = ['127.0.0.1:53']\n# server_names = ['a']\n"
	cat := Catalog{Fields: []Field{
		{Path: "listen_addresses", Type: TypeStringList},
		{Path: "server_names", Type: TypeStringList},
	}}
	cur := CurrentValues(src, cat)
	if !cur["listen_addresses"].Present {
		t.Fatal("listen present")
	}
	if cur["server_names"].Present {
		t.Fatal("server_names should be commented")
	}
}

func TestEncodeLists(t *testing.T) {
	t.Parallel()
	got := encodeTOML([]any{"a", "b"}, TypeStringList)
	if got != "['a', 'b']" {
		t.Fatal(got)
	}
	raw, _ := json.Marshal(true)
	s, err := encodePatchValue(raw, Catalog{Fields: []Field{{Path: "x", Type: TypeBool}}}, "x")
	if err != nil || s != "true" {
		t.Fatalf("%s %v", s, err)
	}
}
