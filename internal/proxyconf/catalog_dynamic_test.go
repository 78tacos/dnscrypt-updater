package proxyconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCatalogPrefersInstalledExample(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "listen_addresses = ['127.0.0.1:53']\nbrand_new_flag = true\n"
	if err := os.WriteFile(filepath.Join(dir, ExampleTOMLName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ExampleTagName), []byte("v2.2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := LoadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Source != CatalogSourceInstall {
		t.Fatalf("source %q", cat.Source)
	}
	if cat.UpstreamTag != "2.2.0" {
		t.Fatalf("tag %q", cat.UpstreamTag)
	}
	f, ok := cat.FieldByPath("brand_new_flag")
	if !ok {
		t.Fatal("installed example key missing")
	}
	if f.Type != TypeBool {
		t.Fatalf("type %s", f.Type)
	}
	if _, ok := cat.FieldByPath("monitoring_ui.enabled"); ok {
		t.Fatal("embedded-only key should not come from the short installed example")
	}
}

func TestLoadCatalogFallsBackWhenExampleMissingOrInvalid(t *testing.T) {
	t.Parallel()
	missing, err := LoadCatalog(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if missing.Source != CatalogSourceEmbedded || missing.UpstreamTag != "2.1.18" {
		t.Fatalf("missing: source %q tag %q", missing.Source, missing.UpstreamTag)
	}
	if _, ok := missing.FieldByPath("monitoring_ui.enabled"); !ok {
		t.Fatal("embedded catalog missing monitoring_ui.enabled")
	}

	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, ExampleTOMLName), []byte("this is not toml {{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, ExampleTagName), []byte("9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCatalog(bad)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != CatalogSourceEmbedded || got.UpstreamTag != "2.1.18" {
		t.Fatalf("invalid example: source %q tag %q", got.Source, got.UpstreamTag)
	}

	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, ExampleTOMLName), []byte("\n# nothing here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = LoadCatalog(empty)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != CatalogSourceEmbedded {
		t.Fatalf("empty example source %q", got.Source)
	}
}

func TestLoadCatalogReloadsAfterExampleChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeExample := func(body, tag string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, ExampleTOMLName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ExampleTagName), []byte(tag+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeExample("only_in_one = 1\n", "1.0.0")
	first, err := LoadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeExample("only_in_two = 'x'\n", "2.0.0")
	second, err := LoadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := first.FieldByPath("only_in_one"); !ok {
		t.Fatal("first catalog missing only_in_one")
	}
	if _, ok := first.FieldByPath("only_in_two"); ok {
		t.Fatal("first catalog cached the later key")
	}
	if _, ok := second.FieldByPath("only_in_two"); !ok {
		t.Fatal("second catalog missing only_in_two")
	}
	if second.UpstreamTag != "2.0.0" {
		t.Fatalf("tag %q", second.UpstreamTag)
	}
	again, err := LoadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.UpstreamTag != second.UpstreamTag || len(again.Fields) != len(second.Fields) {
		t.Fatal("reload did not match the current example")
	}
}

func TestInferTypeFromExample(t *testing.T) {
	t.Parallel()
	src := `
flag = true
count = 3
ratio = 1.5
sci = 1e2
name = 'alpha'
bare = hello
words = ['a', 'b']
ports = [1, 2]
obj = { a = 1 }
routes = [{ server_name = 'a' }]
empty = []
`
	cat, err := parseExampleTOML(src, map[string]string{
		// A key the vendored config describes keeps that type even when the value looks boolean.
		"flag": TypeString,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"flag":   TypeString,
		"count":  TypeInt,
		"ratio":  TypeFloat,
		"sci":    TypeFloat,
		"name":   TypeString,
		"bare":   TypeString,
		"words":  TypeStringList,
		"ports":  TypeIntList,
		"obj":    TypeTable,
		"routes": TypeInlineTableList,
		"empty":  TypeStringList,
	}
	for path, typ := range want {
		f, ok := cat.FieldByPath(path)
		if !ok {
			t.Fatalf("missing %s", path)
		}
		if f.Type != typ {
			t.Fatalf("%s type %s want %s", path, f.Type, typ)
		}
	}
	// Without a config.go type, the same boolean value is a bool.
	untyped, err := parseExampleTOML("flag = true\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := untyped.FieldByPath("flag")
	if f.Type != TypeBool {
		t.Fatalf("untyped flag %s", f.Type)
	}
}

func TestWithUnknownLiveOtherSection(t *testing.T) {
	t.Parallel()
	cat := Catalog{
		UpstreamTag: "2.1.18",
		Fields: []Field{{
			Path: "listen_addresses", Key: "listen_addresses", Type: TypeStringList, SectionTitle: "Global",
		}},
	}
	src := "# hidden = true\nlisten_addresses = ['127.0.0.1:53']\ncustom_knob = 4\n\n[extra]\nsecret = 'z'\n"
	out := cat.WithUnknownLive(src)
	if _, ok := out.FieldByPath("listen_addresses"); !ok {
		t.Fatal("known key dropped")
	}
	knob, ok := out.FieldByPath("custom_knob")
	if !ok || knob.Section != OtherSectionID || knob.Type != TypeInt {
		t.Fatalf("knob %+v", knob)
	}
	secret, ok := out.FieldByPath("extra.secret")
	if !ok || secret.Section != OtherSectionID || secret.Type != TypeString {
		t.Fatalf("secret %+v", secret)
	}
	hidden, ok := out.FieldByPath("hidden")
	if !ok || !hidden.Commented || hidden.Type != TypeBool {
		t.Fatalf("hidden %+v", hidden)
	}
	foundOther := false
	for _, s := range out.Sections {
		if s.ID == OtherSectionID && s.Title == OtherSectionTitle {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatal("missing Other section")
	}
	if len(cat.Fields) != 1 {
		t.Fatal("original catalog was mutated")
	}
}

func TestAnnotateNewBadges(t *testing.T) {
	t.Parallel()
	base := Catalog{UpstreamTag: "2.1.18", Fields: []Field{{Path: "a", Key: "a"}, {Path: "b", Key: "b"}}}
	next := Catalog{UpstreamTag: "2.1.19", Fields: []Field{{Path: "a", Key: "a"}, {Path: "b", Key: "b"}, {Path: "c", Key: "c"}}}

	marked, snap, changed := AnnotateNew(base, CatalogSnapshot{})
	if !changed {
		t.Fatal("baseline should persist")
	}
	for _, f := range marked.Fields {
		if f.NewIn != "" {
			t.Fatalf("baseline badge on %s", f.Path)
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "catalog-snapshot.json")
	if err := SaveCatalogSnapshot(path, snap); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCatalogSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	marked, snap, changed = AnnotateNew(next, loaded)
	if !changed {
		t.Fatal("version change should update the snapshot")
	}
	byPath := map[string]Field{}
	for _, f := range marked.Fields {
		byPath[f.Path] = f
	}
	if byPath["a"].NewIn != "" || byPath["b"].NewIn != "" {
		t.Fatalf("old keys badged: %+v", byPath)
	}
	if byPath["c"].NewIn != "2.1.19" {
		t.Fatalf("new key badge %q", byPath["c"].NewIn)
	}
	if err := SaveCatalogSnapshot(path, snap); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadCatalogSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	again, snap2, changed := AnnotateNew(next, loaded)
	if changed {
		t.Fatal("same catalog should not rewrite the snapshot")
	}
	for _, f := range again.Fields {
		if f.Path == "c" && f.NewIn != "2.1.19" {
			t.Fatalf("badge dropped on reload: %+v", f)
		}
	}
	if snap2.PriorVersion != "2.1.18" {
		t.Fatalf("prior %q", snap2.PriorVersion)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "2.1.18") {
		t.Fatalf("snapshot %s", raw)
	}
}
