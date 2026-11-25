package proxyconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CatalogSnapshot is the previous catalog's version and key set, plus the
// catalog currently on disk. New-in badges are keys in the current set that
// were not in PriorKeys.
type CatalogSnapshot struct {
	Version      string   `json:"version"`
	Keys         []string `json:"keys"`
	PriorVersion string   `json:"prior_version,omitempty"`
	PriorKeys    []string `json:"prior_keys,omitempty"`
}

// LoadCatalogSnapshot reads a snapshot file. A missing file is an empty snapshot.
func LoadCatalogSnapshot(path string) (CatalogSnapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return CatalogSnapshot{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CatalogSnapshot{}, nil
		}
		return CatalogSnapshot{}, err
	}
	var snap CatalogSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return CatalogSnapshot{}, err
	}
	sort.Strings(snap.Keys)
	sort.Strings(snap.PriorKeys)
	return snap, nil
}

// SaveCatalogSnapshot writes the snapshot. An empty path is a no-op.
func SaveCatalogSnapshot(path string, snap CatalogSnapshot) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	sort.Strings(snap.Keys)
	sort.Strings(snap.PriorKeys)
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AnnotateNew marks fields that were added since snap's prior key set.
// The first observation records a baseline and shows no badges. A later
// catalog (different version or key set) becomes current, and the previous
// key set is what "new" is compared against. changed is true when snap was updated.
func AnnotateNew(cat Catalog, snap CatalogSnapshot) (Catalog, CatalogSnapshot, bool) {
	keys := catalogKeys(cat)
	if snap.Version == "" && len(snap.Keys) == 0 && snap.PriorVersion == "" && len(snap.PriorKeys) == 0 {
		return cat, CatalogSnapshot{Version: cat.UpstreamTag, Keys: keys}, true
	}
	changed := false
	if snap.Version != cat.UpstreamTag || !sameStrings(snap.Keys, keys) {
		snap = CatalogSnapshot{
			Version:      cat.UpstreamTag,
			Keys:         keys,
			PriorVersion: snap.Version,
			PriorKeys:    append([]string{}, snap.Keys...),
		}
		changed = true
	}
	if snap.PriorVersion == "" && len(snap.PriorKeys) == 0 {
		return cat, snap, changed
	}
	prior := make(map[string]struct{}, len(snap.PriorKeys))
	for _, k := range snap.PriorKeys {
		prior[k] = struct{}{}
	}
	fields := append([]Field{}, cat.Fields...)
	for i := range fields {
		if _, ok := prior[fields[i].Path]; !ok {
			fields[i].NewIn = cat.UpstreamTag
		}
	}
	cat.Fields = fields
	return cat, snap, changed
}

func catalogKeys(cat Catalog) []string {
	seen := make(map[string]struct{}, len(cat.Fields))
	keys := make([]string, 0, len(cat.Fields))
	for _, f := range cat.Fields {
		if f.Path == "" || f.Section == OtherSectionID {
			continue
		}
		if _, ok := seen[f.Path]; ok {
			continue
		}
		seen[f.Path] = struct{}{}
		keys = append(keys, f.Path)
	}
	sort.Strings(keys)
	return keys
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
