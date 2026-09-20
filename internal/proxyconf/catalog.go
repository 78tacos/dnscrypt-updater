package proxyconf

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

//go:embed upstream/example-dnscrypt-proxy.toml
//go:embed upstream/config.go.txt
//go:embed upstream/VERSION
//go:embed upstream/example-*.txt
var upstreamFS embed.FS

var (
	catalogOnce sync.Once
	catalogVal  Catalog
	catalogErr  error
)

// LoadCatalog parses the vendored upstream example toml and config.go types.
func LoadCatalog() (Catalog, error) {
	catalogOnce.Do(func() {
		catalogVal, catalogErr = parseVendored()
	})
	return catalogVal, catalogErr
}

func parseVendored() (Catalog, error) {
	tomlBytes, err := upstreamFS.ReadFile("upstream/example-dnscrypt-proxy.toml")
	if err != nil {
		return Catalog{}, err
	}
	goBytes, err := upstreamFS.ReadFile("upstream/config.go.txt")
	if err != nil {
		return Catalog{}, err
	}
	tagBytes, err := upstreamFS.ReadFile("upstream/VERSION")
	if err != nil {
		return Catalog{}, err
	}
	structs := parseGoTypes(string(goBytes))
	types := flattenGoTypes(structs)
	cat, err := parseExampleTOML(string(tomlBytes), types)
	if err != nil {
		return Catalog{}, err
	}
	cat.UpstreamTag = strings.TrimSpace(string(tagBytes))
	sum := sha256.Sum256(tomlBytes)
	cat.ExampleSHA256 = hex.EncodeToString(sum[:])
	if err := validateCatalog(cat); err != nil {
		return cat, err
	}
	return cat, nil
}

func validateCatalog(cat Catalog) error {
	need := []string{"listen_addresses", "server_names", "require_nolog", "blocked_names.blocked_names_file", "monitoring_ui.enabled"}
	for _, p := range need {
		if _, ok := cat.FieldByPath(p); !ok {
			return fmt.Errorf("catalog missing required path %s (upstream changed? re-run go generate)", p)
		}
	}
	for _, p := range easyPaths {
		if _, ok := cat.FieldByPath(p); !ok {
			return fmt.Errorf("easy path %s not in catalog", p)
		}
	}
	return nil
}

// ExampleListFile returns the vendored example for a companion list file.
func ExampleListFile(name string) ([]byte, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("invalid example file name")
	}
	return upstreamFS.ReadFile("upstream/" + name)
}

// UpstreamTag is the pinned DNSCrypt/dnscrypt-proxy release.
func UpstreamTag() string {
	b, err := upstreamFS.ReadFile("upstream/VERSION")
	if err != nil {
		return GeneratedUpstreamTag
	}
	return strings.TrimSpace(string(b))
}
