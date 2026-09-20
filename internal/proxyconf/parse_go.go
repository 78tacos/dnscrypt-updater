package proxyconf

import (
	"regexp"
	"strings"
	"unicode"
)

type goField struct {
	TomlKey  string
	TypeName string
	Kind     string // bool, int, float, string, string_list, int_list, inline_table_list, table, table_map, unknown
	Elem     string
}

type goStruct struct {
	Name   string
	Fields []goField
}

var (
	structRe = regexp.MustCompile(`(?s)type\s+([A-Za-z0-9_]+)\s+struct\s*\{`)
	fieldRe  = regexp.MustCompile("^\t([A-Za-z0-9_]+)\\s+([^`\\n]+?)(?:\\s+`([^`]*)`)?\\s*$")
	tomlTag  = regexp.MustCompile(`toml:"([^"]+)"`)
)

func parseGoTypes(src string) map[string]goStruct {
	out := make(map[string]goStruct)
	for _, loc := range structRe.FindAllStringSubmatchIndex(src, -1) {
		name := src[loc[2]:loc[3]]
		bodyStart := loc[1]
		body, ok := structBody(src, bodyStart)
		if !ok {
			continue
		}
		st := goStruct{Name: name}
		for _, line := range strings.Split(body, "\n") {
			if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			m := fieldRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			typeName := strings.TrimSpace(m[2])
			key := ""
			if m[3] != "" {
				if tm := tomlTag.FindStringSubmatch(m[3]); tm != nil {
					key = tm[1]
				}
			}
			if key == "" {
				key = strings.ToLower(m[1])
			}
			st.Fields = append(st.Fields, classifyGoField(key, typeName))
		}
		out[name] = st
	}
	return out
}

func structBody(src string, openBraceAt int) (string, bool) {
	// openBraceAt is index after "struct {"
	i := strings.Index(src[openBraceAt-1:], "{")
	if i < 0 {
		return "", false
	}
	start := openBraceAt - 1 + i + 1
	depth := 1
	for p := start; p < len(src); p++ {
		switch src[p] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start:p], true
			}
		}
	}
	return "", false
}

func classifyGoField(key, typeName string) goField {
	f := goField{TomlKey: key, TypeName: strings.TrimSpace(typeName)}
	t := strings.TrimSpace(typeName)
	t = strings.TrimPrefix(t, "*")
	switch {
	case t == "bool":
		f.Kind = TypeBool
	case t == "string":
		f.Kind = TypeString
	case t == "int" || t == "int32" || t == "int64" || t == "uint" || t == "uint32" || t == "uint64":
		f.Kind = TypeInt
	case t == "float32" || t == "float64":
		f.Kind = TypeFloat
	case strings.HasPrefix(t, "[]"):
		elem := strings.TrimPrefix(t, "[]")
		f.Elem = elem
		if elem == "string" {
			f.Kind = TypeStringList
		} else if isIntish(elem) {
			f.Kind = TypeIntList
		} else if unicode.IsUpper(rune(elem[0])) {
			f.Kind = TypeInlineTableList
		} else {
			f.Kind = TypeStringList
		}
	case strings.HasPrefix(t, "map["):
		f.Kind = TypeTableMap
		if i := strings.Index(t, "]"); i >= 0 {
			f.Elem = t[i+1:]
		}
	case len(t) > 0 && unicode.IsUpper(rune(t[0])):
		f.Kind = TypeTable
		f.Elem = t
	default:
		f.Kind = TypeUnknown
	}
	return f
}

func isIntish(t string) bool {
	switch t {
	case "int", "int32", "int64", "uint", "uint16", "uint32", "uint64":
		return true
	default:
		return false
	}
}

func flattenGoTypes(structs map[string]goStruct) map[string]string {
	out := make(map[string]string)
	cfg, ok := structs["Config"]
	if !ok {
		return out
	}
	walkStruct(structs, cfg, "", out)
	return out
}

func walkStruct(structs map[string]goStruct, st goStruct, prefix string, out map[string]string) {
	for _, f := range st.Fields {
		path := f.TomlKey
		if prefix != "" {
			path = prefix + "." + f.TomlKey
		}
		out[path] = f.Kind
		if f.Kind == TypeTable {
			if nested, ok := structs[f.Elem]; ok {
				walkStruct(structs, nested, path, out)
			}
		}
		if f.Kind == TypeTableMap {
			if nested, ok := structs[f.Elem]; ok {
				// Instance tables: sources.NAME.key
				walkStruct(structs, nested, path+".*", out)
			}
		}
	}
}

func lookupType(types map[string]string, path string) string {
	if t, ok := types[path]; ok && t != "" {
		return t
	}
	// sources.public-resolvers.urls -> sources.*.urls
	parts := strings.Split(path, ".")
	if len(parts) >= 3 {
		wildcard := parts[0] + ".*." + strings.Join(parts[2:], ".")
		if t, ok := types[wildcard]; ok && t != "" {
			return t
		}
	}
	if len(parts) >= 2 {
		wildcard := parts[0] + ".*." + parts[len(parts)-1]
		if t, ok := types[wildcard]; ok && t != "" {
			return t
		}
	}
	return ""
}
