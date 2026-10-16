package proxyconf

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Value is the live state of one toml key.
type Value struct {
	Path    string `json:"path"`
	Present bool   `json:"present"`
	Raw     string `json:"raw"`
	Decoded any    `json:"value"`
	Unknown bool   `json:"unknown,omitempty"`
	Comment bool   `json:"commented"`
}

// Patch sets or comments one key. Value is JSON (bool, number, string, array).
type Patch struct {
	Path    string          `json:"path"`
	Enabled bool            `json:"enabled"`
	Value   json.RawMessage `json:"value,omitempty"`
}

type keySpan struct {
	path      string
	section   string
	key       string
	start     int
	end       int
	commented bool
	raw       string
}

type doc struct {
	lines []string
	spans []keySpan
}

func parseDoc(src string) *doc {
	d := &doc{lines: splitKeepNL(src)}
	section := ""
	i := 0
	for i < len(d.lines) {
		trim := strings.TrimSpace(stripNL(d.lines[i]))
		if t, commented, ok := tableHeader(trim); ok {
			if !commented {
				section = t
			}
			i++
			continue
		}
		key, val, commented, end, ok := readKey(d.lines, i)
		if !ok {
			i++
			continue
		}
		d.spans = append(d.spans, keySpan{
			path:      fieldPath(section, key),
			section:   section,
			key:       key,
			start:     i,
			end:       end,
			commented: commented,
			raw:       val,
		})
		i = end + 1
	}
	return d
}

func (d *doc) String() string {
	return strings.Join(d.lines, "")
}

func (d *doc) values(cat Catalog) map[string]Value {
	known := map[string]bool{}
	for _, f := range cat.Fields {
		known[f.Path] = true
	}
	out := make(map[string]Value, len(d.spans))
	for _, sp := range d.spans {
		v := Value{
			Path:    sp.path,
			Present: !sp.commented,
			Raw:     sp.raw,
			Comment: sp.commented,
			Unknown: !known[sp.path],
		}
		typ := TypeUnknown
		if f, ok := cat.FieldByPath(sp.path); ok {
			typ = f.Type
		}
		if dec, err := decodeTOMLScalar(sp.raw, typ); err == nil {
			v.Decoded = dec
		} else {
			v.Decoded = strings.TrimSpace(sp.raw)
		}
		out[sp.path] = v
	}
	return out
}

func (d *doc) find(path string) (int, bool) {
	for i, sp := range d.spans {
		if sp.path == path {
			return i, true
		}
	}
	return -1, false
}

func (d *doc) applyPatch(p Patch, cat Catalog) error {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return fmt.Errorf("empty patch path")
	}
	enc, err := encodePatchValue(p.Value, cat, path)
	if err != nil {
		return err
	}
	if i, ok := d.find(path); ok {
		sp := d.spans[i]
		line := formatAssignment(sp.key, enc)
		if !p.Enabled {
			line = "# " + line
		}
		d.replaceSpan(i, []string{ensureNL(line)})
		return nil
	}
	if !p.Enabled && enc == "" {
		return nil
	}
	section, key := splitPath(path)
	line := formatAssignment(key, enc)
	if !p.Enabled {
		line = "# " + line
	}
	return d.insertInSection(section, ensureNL(line), path, key, !p.Enabled, enc)
}

func (d *doc) replaceSpan(idx int, newlines []string) {
	sp := d.spans[idx]
	head := append([]string{}, d.lines[:sp.start]...)
	tail := append([]string{}, d.lines[sp.end+1:]...)
	d.lines = append(head, append(newlines, tail...)...)
	d.rebuild()
}

func (d *doc) insertInSection(section, line, path, key string, commented bool, raw string) error {
	insertAt := d.sectionInsertIndex(section)
	if insertAt < 0 {
		if section != "" {
			d.lines = append(d.lines, "\n", ensureNL("["+section+"]"), "\n")
		}
		d.lines = append(d.lines, line)
		d.rebuild()
		return nil
	}
	head := append([]string{}, d.lines[:insertAt]...)
	tail := append([]string{}, d.lines[insertAt:]...)
	d.lines = append(head, append([]string{line}, tail...)...)
	d.rebuild()
	return nil
}

func (d *doc) sectionInsertIndex(section string) int {
	header := -1
	for i, ln := range d.lines {
		trim := strings.TrimSpace(stripNL(ln))
		t, commented, ok := tableHeader(trim)
		if !ok || commented {
			continue
		}
		if section == "" {
			// root: before first table
			return i
		}
		if t == section {
			header = i
			continue
		}
		if header >= 0 && t != section && (section == "" || !strings.HasPrefix(t, section+".")) {
			return i
		}
	}
	if section == "" {
		return 0
	}
	if header >= 0 {
		return len(d.lines)
	}
	return -1
}

func (d *doc) rebuild() {
	src := d.String()
	*d = *parseDoc(src)
}

func splitPath(path string) (section, key string) {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

func formatAssignment(key, enc string) string {
	return key + " = " + enc
}

func ensureNL(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func encodePatchValue(raw json.RawMessage, cat Catalog, path string) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if f, ok := cat.FieldByPath(path); ok && f.DefaultRaw != "" {
			return f.DefaultRaw, nil
		}
		return `''`, nil
	}
	typ := TypeUnknown
	if f, ok := cat.FieldByPath(path); ok {
		typ = f.Type
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return encodeTOML(v, typ), nil
}

func encodeTOML(v any, typ string) string {
	switch val := v.(type) {
	case nil:
		return `''`
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if typ == TypeInt || val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case json.Number:
		return val.String()
	case string:
		return quoteTOML(val)
	case []any:
		if typ == TypeInlineTableList {
			return encodeInlineTables(val)
		}
		parts := make([]string, 0, len(val))
		for _, x := range val {
			parts = append(parts, encodeTOML(x, TypeString))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		return encodeInlineTable(val)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return quoteTOML(fmt.Sprint(v))
		}
		var generic any
		_ = json.Unmarshal(b, &generic)
		return encodeTOML(generic, typ)
	}
}

func encodeInlineTables(arr []any) string {
	parts := make([]string, 0, len(arr))
	for _, x := range arr {
		m, ok := x.(map[string]any)
		if !ok {
			parts = append(parts, encodeTOML(x, TypeUnknown))
			continue
		}
		parts = append(parts, encodeInlineTable(m))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func encodeInlineTable(m map[string]any) string {
	keys := make([]string, 0, len(m))
	// stable-ish preferred order
	prefer := []string{"server_name", "via", "stamp"}
	seen := map[string]bool{}
	for _, k := range prefer {
		if _, ok := m[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for k := range m {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+" = "+encodeTOML(m[k], TypeUnknown))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func quoteTOML(s string) string {
	if !strings.ContainsAny(s, "'\n") {
		return "'" + s + "'"
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// ApplyPatches returns a new toml document with surgical key updates.
// Unknown keys and comments outside patched spans are preserved.
func ApplyPatches(src string, patches []Patch, cat Catalog) (string, error) {
	d := parseDoc(src)
	for _, p := range patches {
		if err := d.applyPatch(p, cat); err != nil {
			return "", err
		}
	}
	return d.String(), nil
}

// CurrentValues reads live toml keys.
func CurrentValues(src string, cat Catalog) map[string]Value {
	return parseDoc(src).values(cat)
}
