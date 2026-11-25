package proxyconf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// tryInstalledCatalog reads the example copied into the install directory.
// ok is false when the file is missing or does not parse.
func tryInstalledCatalog(installDir string) (Catalog, bool) {
	installDir = strings.TrimSpace(installDir)
	if installDir == "" {
		return Catalog{}, false
	}
	b, err := os.ReadFile(filepath.Join(installDir, ExampleTOMLName))
	if err != nil {
		return Catalog{}, false
	}
	b = normalizeTOMLNewlines(b)
	goBytes, err := upstreamFS.ReadFile("upstream/config.go.txt")
	if err != nil {
		return Catalog{}, false
	}
	types := flattenGoTypes(parseGoTypes(string(goBytes)))
	cat, err := parseExampleTOML(string(b), types)
	if err != nil {
		return Catalog{}, false
	}
	cat.Source = CatalogSourceInstall
	cat.UpstreamTag = installedUpstreamTag(installDir, b)
	sum := sha256.Sum256(b)
	cat.ExampleSHA256 = hex.EncodeToString(sum[:])
	return cat, true
}

func normalizeTOMLNewlines(b []byte) []byte {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte("\r"), []byte("\n"))
	return b
}

func installedUpstreamTag(dir string, normalized []byte) string {
	if b, err := os.ReadFile(filepath.Join(dir, ExampleTagName)); err == nil {
		v := strings.TrimSpace(string(b))
		v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
		if v != "" {
			return v
		}
	}
	if tag, ok := embeddedTagForMatchingExample(normalized); ok {
		return tag
	}
	return "installed"
}

func embeddedTagForMatchingExample(normalized []byte) (string, bool) {
	emb, err := upstreamFS.ReadFile("upstream/" + ExampleTOMLName)
	if err != nil {
		return "", false
	}
	emb = normalizeTOMLNewlines(emb)
	if sha256.Sum256(emb) != sha256.Sum256(normalized) {
		return "", false
	}
	tag, err := upstreamFS.ReadFile("upstream/VERSION")
	if err != nil {
		return GeneratedUpstreamTag, true
	}
	return strings.TrimSpace(string(tag)), true
}

// WithUnknownLive adds an Other section for keys in the live toml that this
// catalog does not already describe. Known keys are left as they are.
func (c Catalog) WithUnknownLive(src string) Catalog {
	if strings.TrimSpace(src) == "" {
		return c
	}
	known := make(map[string]bool, len(c.Fields))
	for _, f := range c.Fields {
		if f.Path != "" {
			known[f.Path] = true
		}
	}
	var extra []Field
	for _, sp := range parseDoc(src).spans {
		if sp.path == "" || known[sp.path] {
			continue
		}
		known[sp.path] = true
		typ := inferTypeFromExample(sp.raw)
		f := Field{
			Path:         sp.path,
			Key:          sp.key,
			Section:      OtherSectionID,
			SectionTitle: OtherSectionTitle,
			Type:         typ,
			Help:         "This key is in your dnscrypt-proxy.toml and is not in the example catalog.",
			DefaultRaw:   strings.TrimSpace(sp.raw),
			Commented:    sp.commented,
		}
		if dec, err := decodeTOMLScalar(sp.raw, typ); err == nil {
			f.Default = dec
		}
		extra = append(extra, f)
	}
	if len(extra) == 0 {
		return c
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Path < extra[j].Path })
	sections := append([]Section{}, c.Sections...)
	hasOther := false
	for _, s := range sections {
		if s.ID == OtherSectionID {
			hasOther = true
			break
		}
	}
	if !hasOther {
		sections = append(sections, Section{
			ID:    OtherSectionID,
			Title: OtherSectionTitle,
			Help:  "Keys in your config that the example catalog does not describe.",
		})
	}
	fields := append([]Field{}, c.Fields...)
	c.Sections = sections
	c.Fields = append(fields, extra...)
	return c
}
