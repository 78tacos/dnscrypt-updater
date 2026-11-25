package proxyconf

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"strings"
)

//go:embed upstream/example-dnscrypt-proxy.toml
//go:embed upstream/config.go.txt
//go:embed upstream/VERSION
//go:embed upstream/example-*.txt
var upstreamFS embed.FS

const (
	// ExampleTOMLName is the upstream example apply copies into the install directory.
	ExampleTOMLName = "example-dnscrypt-proxy.toml"
	// ExampleTagName is the dnscrypt-proxy version written next to that example.
	ExampleTagName = "example-dnscrypt-proxy.tag"

	// CatalogSourceEmbedded is the copy pinned under upstream/.
	CatalogSourceEmbedded = "embedded"
	// CatalogSourceInstall is example-dnscrypt-proxy.toml from the install directory.
	CatalogSourceInstall = "install"
)

// LoadCatalog builds the settings catalog.
//
// It prefers example-dnscrypt-proxy.toml in installDir (the file apply copies
// on every update). A missing file, a read error, or a file that does not
// parse falls back to the embedded upstream copy. The result is not cached;
// call LoadCatalog again after an update.
func LoadCatalog(installDir string) (Catalog, error) {
	if cat, ok := tryInstalledCatalog(installDir); ok {
		return cat, nil
	}
	cat, err := parseVendored()
	if err != nil {
		return Catalog{}, err
	}
	cat.Source = CatalogSourceEmbedded
	return cat, nil
}

func parseVendored() (Catalog, error) {
	tomlBytes, err := upstreamFS.ReadFile("upstream/example-dnscrypt-proxy.toml")
	if err != nil {
		return Catalog{}, err
	}
	tomlBytes = bytes.ReplaceAll(tomlBytes, []byte("\r\n"), []byte("\n"))
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
