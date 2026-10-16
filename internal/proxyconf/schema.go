package proxyconf

import "strings"

// Field types inferred from upstream config.go and the example toml.
const (
	TypeBool            = "bool"
	TypeInt             = "int"
	TypeFloat           = "float"
	TypeString          = "string"
	TypeStringList      = "string_list"
	TypeIntList         = "int_list"
	TypeInlineTableList = "inline_table_list"
	TypeTable           = "table"
	TypeTableMap        = "table_map"
	TypeUnknown         = "unknown"
)

// Field is one dnscrypt-proxy.toml key the UI can edit.
type Field struct {
	Path         string   `json:"path"`
	Key          string   `json:"key"`
	Section      string   `json:"section"`
	SectionTitle string   `json:"section_title"`
	Type         string   `json:"type"`
	Help         string   `json:"help"`
	Default      any      `json:"default,omitempty"`
	DefaultRaw   string   `json:"default_raw"`
	Commented    bool     `json:"commented"`
	Examples     []string `json:"examples,omitempty"`
	RelatedFile  string   `json:"related_file,omitempty"`
	ExampleFile  string   `json:"example_file,omitempty"`
	Warning      bool     `json:"warning"`
	Easy         bool     `json:"easy"`
}

// Section groups fields in the example toml.
type Section struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Help  string `json:"help,omitempty"`
}

// Catalog is the generated-from-upstream settings surface.
type Catalog struct {
	UpstreamTag    string    `json:"upstream_tag"`
	ExampleSHA256  string    `json:"example_sha256"`
	Sections       []Section `json:"sections"`
	Fields         []Field   `json:"fields"`
	CompanionFiles []string  `json:"companion_files"`
}

// FieldByPath returns a field or false.
func (c Catalog) FieldByPath(path string) (Field, bool) {
	for _, f := range c.Fields {
		if f.Path == path {
			return f, true
		}
	}
	return Field{}, false
}

// Paths returns every catalog path.
func (c Catalog) Paths() []string {
	out := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		out = append(out, f.Path)
	}
	return out
}

func fieldPath(section, key string) string {
	section = strings.TrimSpace(section)
	key = strings.TrimSpace(key)
	if section == "" {
		return key
	}
	return section + "." + key
}
