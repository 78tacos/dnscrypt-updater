package proxyconf

import (
	"strings"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	t.Parallel()
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if cat.UpstreamTag != "2.1.18" {
		t.Fatalf("tag %q", cat.UpstreamTag)
	}
	if cat.ExampleSHA256 == "" || len(cat.ExampleSHA256) != 64 {
		t.Fatalf("sha %q", cat.ExampleSHA256)
	}
	if GeneratedExampleSHA256 == "" {
		t.Fatal("run go generate ./internal/proxyconf")
	}
	if cat.ExampleSHA256 != GeneratedExampleSHA256 {
		t.Fatalf("vendored toml sha %s != generated %s (re-run go generate)", cat.ExampleSHA256, GeneratedExampleSHA256)
	}
	need := []string{
		"listen_addresses",
		"server_names",
		"require_nolog",
		"blocked_names.blocked_names_file",
		"monitoring_ui.enabled",
		"anonymized_dns.routes",
		"sources.public-resolvers.urls",
	}
	for _, p := range need {
		f, ok := cat.FieldByPath(p)
		if !ok {
			t.Fatalf("missing %s (fields=%d)", p, len(cat.Fields))
		}
		if f.Help == "" && p != "sources.public-resolvers.urls" {
			t.Fatalf("%s has empty help", p)
		}
		if strings.Contains(f.Help, "This is an example configuration file") {
			t.Fatalf("%s help leaked file header: %q", p, f.Help)
		}
		if strings.Contains(f.Help, "######") {
			t.Fatalf("%s help leaked hash banner: %q", p, f.Help)
		}
	}
	listen, _ := cat.FieldByPath("listen_addresses")
	if listen.Type != TypeStringList {
		t.Fatalf("listen type %s", listen.Type)
	}
	if listen.Commented {
		t.Fatal("listen_addresses should be active in the example")
	}
	names, _ := cat.FieldByPath("server_names")
	if !names.Commented {
		t.Fatal("server_names should be commented in the example")
	}
	if !strings.Contains(names.Help, "List of servers to use") {
		t.Fatalf("server_names help too short: %q", names.Help)
	}
	if listen.Easy != true || names.Easy != true {
		t.Fatal("easy flags")
	}
	bn, _ := cat.FieldByPath("blocked_names.blocked_names_file")
	if bn.RelatedFile != "blocked-names.txt" {
		t.Fatalf("related %q", bn.RelatedFile)
	}
	mon, _ := cat.FieldByPath("monitoring_ui.enabled")
	if mon.Type != TypeBool {
		t.Fatalf("monitoring type %s", mon.Type)
	}
	if ValidatePresets(cat) != nil {
		t.Fatal(ValidatePresets(cat))
	}
	if len(cat.Fields) < 80 {
		t.Fatalf("too few fields: %d", len(cat.Fields))
	}
}

func TestParseGoTypes(t *testing.T) {
	t.Parallel()
	src := `
type Config struct {
	ListenAddresses []string ` + "`toml:\"listen_addresses\"`" + `
	Cache bool
	QueryLog QueryLogConfig ` + "`toml:\"query_log\"`" + `
	SourcesConfig map[string]SourceConfig ` + "`toml:\"sources\"`" + `
}
type QueryLogConfig struct {
	File string
	IgnoredQtypes []string ` + "`toml:\"ignored_qtypes\"`" + `
}
type SourceConfig struct {
	URLs []string
	MinisignKeyStr string ` + "`toml:\"minisign_key\"`" + `
}
`
	structs := parseGoTypes(src)
	types := flattenGoTypes(structs)
	if types["listen_addresses"] != TypeStringList {
		t.Fatalf("%v", types)
	}
	if types["cache"] != TypeBool {
		t.Fatalf("cache %q", types["cache"])
	}
	if types["query_log.file"] != TypeString {
		t.Fatalf("query_log.file %q", types["query_log.file"])
	}
	if types["sources.*.urls"] != TypeStringList {
		t.Fatalf("sources urls %q", types["sources.*.urls"])
	}
	if lookupType(types, "sources.public-resolvers.minisign_key") != TypeString {
		t.Fatalf("minisign lookup %v", types)
	}
}

func TestBannerAndTable(t *testing.T) {
	t.Parallel()
	title, ok := bannerTitle("#                             Global settings                                  #")
	if !ok || title != "Global settings" {
		t.Fatalf("%q %v", title, ok)
	}
	name, commented, ok := tableHeader("[monitoring_ui]")
	if !ok || commented || name != "monitoring_ui" {
		t.Fatal(name, commented, ok)
	}
	name, commented, ok = tableHeader("# [sources.odoh-servers]")
	if !ok || !commented || name != "sources.odoh-servers" {
		t.Fatal(name, commented, ok)
	}
}

func TestExtractExamples(t *testing.T) {
	t.Parallel()
	help := "Example with both IPv4 and IPv6:\nlisten_addresses = ['127.0.0.1:53', '[::1]:53']\nUse `0.0.0.0:53`"
	got := extractExamples(help, "['127.0.0.1:53']")
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "127.0.0.1:53") {
		t.Fatal(got)
	}
}
