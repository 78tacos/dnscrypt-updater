package proxyconf

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

func parseExampleTOML(src string, types map[string]string) (Catalog, error) {
	lines := splitKeepNL(src)
	cat := Catalog{Sections: nil, Fields: nil}

	section := ""
	sectionTitle := "Global settings"
	sectionHelp := ""
	help := make([]string, 0, 8)
	seenSection := map[string]string{}

	ensureSection := func() {
		id := section
		title := sectionTitle
		if id != "" && title == "Global settings" {
			title = prettySection(id)
		}
		if prev, ok := seenSection[id]; ok && prev == title {
			return
		}
		if _, ok := seenSection[id]; !ok {
			cat.Sections = append(cat.Sections, Section{ID: id, Title: title, Help: strings.TrimSpace(sectionHelp)})
			seenSection[id] = title
		}
	}

	i := 0
	for i < len(lines) {
		raw := lines[i]
		trim := strings.TrimSpace(stripNL(raw))

		if title, ok := bannerTitle(trim); ok {
			sectionTitle = title
			sectionHelp = ""
			i++
			continue
		}

		if t, commented, ok := tableHeader(trim); ok {
			if !commented {
				section = t
				ensureSection()
				help = help[:0]
			}
			i++
			continue
		}

		if strings.HasPrefix(trim, "##") {
			text := strings.TrimSpace(strings.TrimPrefix(trim, "##"))
			help = append(help, text)
			i++
			continue
		}
		if isHelpComment(trim) {
			text := strings.TrimSpace(strings.TrimPrefix(trim, "#"))
			help = append(help, text)
			i++
			continue
		}

		key, val, commented, end, ok := readKey(lines, i)
		if !ok {
			i++
			continue
		}
		ensureSection()
		path := fieldPath(section, key)
		helpText := strings.TrimSpace(strings.Join(nonEmpty(help), "\n"))
		f := Field{
			Path:         path,
			Key:          key,
			Section:      section,
			SectionTitle: sectionTitleFor(section, sectionTitle, seenSection),
			Type:         inferType(path, val, types),
			Help:         helpText,
			DefaultRaw:   strings.TrimSpace(val),
			Commented:    commented,
			Examples:     extractExamples(helpText, val),
			Warning:      hasWarning(helpText),
		}
		if !commented {
			if d, err := decodeTOMLScalar(val, f.Type); err == nil {
				f.Default = d
			}
		} else if d, err := decodeTOMLScalar(val, f.Type); err == nil {
			f.Default = d
		}
		cat.Fields = append(cat.Fields, f)
		help = help[:0]
		i = end + 1
	}

	rel := relatedByPath()
	easy := make(map[string]bool, len(easyPaths))
	for _, p := range easyPaths {
		easy[p] = true
	}
	for i := range cat.Fields {
		if pair, ok := rel[cat.Fields[i].Path]; ok {
			cat.Fields[i].RelatedFile = pair[0]
			cat.Fields[i].ExampleFile = pair[1]
		}
		cat.Fields[i].Easy = easy[cat.Fields[i].Path]
	}
	cat.CompanionFiles = companionLiveNames()
	return cat, nil
}

func sectionTitleFor(id, current string, seen map[string]string) string {
	if t, ok := seen[id]; ok && t != "" {
		return t
	}
	if id == "" {
		return "Global settings"
	}
	if current != "" {
		return current
	}
	return prettySection(id)
}

func prettySection(id string) string {
	if id == "" {
		return "Global settings"
	}
	parts := strings.Split(id, ".")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "_", " ")
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " / ")
}

func bannerTitle(trim string) (string, bool) {
	if !strings.HasPrefix(trim, "#") {
		return "", false
	}
	inner := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trim, "#")), "#")
	inner = strings.TrimSpace(inner)
	if inner == "" || strings.Contains(inner, "dnscrypt-proxy configuration") {
		return "", false
	}
	if len(trim) < 10 || !strings.Contains(trim, "  ") {
		// require the padded banner style: "#    Title    #"
		if !(strings.HasPrefix(trim, "#  ") && strings.HasSuffix(trim, "  #")) && !strings.Contains(trim, "     ") {
			return "", false
		}
	}
	if strings.ContainsAny(inner, "=[]") {
		return "", false
	}
	if len(inner) < 3 || len(inner) > 60 {
		return "", false
	}
	return inner, true
}

func isHelpComment(trim string) bool {
	if !strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "##") {
		return false
	}
	if _, ok := bannerTitle(trim); ok {
		return false
	}
	if strings.Trim(trim, "# ") == "" {
		return false
	}
	if _, _, ok := tableHeader(trim); ok {
		return false
	}
	body := strings.TrimSpace(strings.TrimPrefix(trim, "#"))
	if eq := strings.Index(body, "="); eq > 0 {
		key := strings.TrimSpace(body[:eq])
		if key != "" && !strings.ContainsAny(key, " \t[]") {
			ok := true
			for _, r := range key {
				if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
					ok = false
					break
				}
			}
			if ok {
				return false // commented-out assignment
			}
		}
	}
	return true
}

func tableHeader(trim string) (name string, commented bool, ok bool) {
	s := trim
	if strings.HasPrefix(s, "#") {
		commented = true
		s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	}
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return "", false, false
	}
	name = strings.TrimSpace(s[1 : len(s)-1])
	if name == "" || strings.Contains(name, " ") {
		return "", false, false
	}
	return name, commented, true
}

func readKey(lines []string, i int) (key, val string, commented bool, end int, ok bool) {
	trim := strings.TrimSpace(stripNL(lines[i]))
	if trim == "" || strings.HasPrefix(trim, "##") {
		return "", "", false, i, false
	}
	line := trim
	if strings.HasPrefix(line, "#") {
		commented = true
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}
	eq := strings.Index(line, "=")
	if eq <= 0 {
		return "", "", false, i, false
	}
	key = strings.TrimSpace(line[:eq])
	if key == "" || strings.ContainsAny(key, " \t[]") {
		return "", "", false, i, false
	}
	for _, r := range key {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return "", "", false, i, false
		}
	}
	rest := strings.TrimSpace(line[eq+1:])
	val, extra, errEnd := readValue(lines, i, rest, commented)
	if extra < i {
		return "", "", false, i, false
	}
	_ = errEnd
	return key, val, commented, extra, true
}

func readValue(lines []string, start int, rest string, commented bool) (val string, end int, incomplete bool) {
	rest = stripInlineComment(rest)
	open := countOpen(rest)
	if open == 0 {
		return strings.TrimSpace(rest), start, false
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(rest))
	end = start
	for j := start + 1; j < len(lines) && open > 0; j++ {
		t := strings.TrimSpace(stripNL(lines[j]))
		if commented {
			if !strings.HasPrefix(t, "#") {
				break
			}
			t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		}
		t = stripInlineComment(t)
		b.WriteByte(' ')
		b.WriteString(t)
		open += countOpen(t)
		end = j
	}
	return strings.TrimSpace(b.String()), end, open != 0
}

func countOpen(s string) int {
	inSQ, inDQ := false, false
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDQ {
			inSQ = !inSQ
			continue
		}
		if c == '"' && !inSQ {
			inDQ = !inDQ
			continue
		}
		if inSQ || inDQ {
			continue
		}
		switch c {
		case '[', '{':
			n++
		case ']', '}':
			n--
		}
	}
	return n
}

func stripInlineComment(s string) string {
	inSQ, inDQ := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDQ {
			inSQ = !inSQ
		} else if c == '"' && !inSQ {
			inDQ = !inDQ
		} else if c == '#' && !inSQ && !inDQ {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

func inferType(path, raw string, types map[string]string) string {
	if t := lookupType(types, path); t != "" && t != TypeTable && t != TypeTableMap {
		return t
	}
	s := strings.TrimSpace(raw)
	switch {
	case s == "true" || s == "false":
		return TypeBool
	case strings.HasPrefix(s, "["):
		if strings.Contains(s, "{") {
			return TypeInlineTableList
		}
		return TypeStringList
	case strings.ContainsAny(s, `"'`):
		return TypeString
	default:
		if _, err := strconv.ParseInt(s, 10, 64); err == nil {
			return TypeInt
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return TypeFloat
		}
		return TypeString
	}
}

func decodeTOMLScalar(raw, typ string) (any, error) {
	s := strings.TrimSpace(raw)
	switch typ {
	case TypeBool:
		return strconv.ParseBool(s)
	case TypeInt:
		return strconv.ParseInt(s, 10, 64)
	case TypeFloat:
		return strconv.ParseFloat(s, 64)
	case TypeString:
		return unquoteTOML(s), nil
	case TypeStringList:
		return parseStringList(s), nil
	case TypeIntList:
		return parseIntList(s), nil
	default:
		if s == "" {
			return nil, strconv.ErrSyntax
		}
		var v any
		if err := json.Unmarshal([]byte(tomlishToJSON(s)), &v); err == nil {
			return v, nil
		}
		return s, nil
	}
}

func unquoteTOML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := splitTOMLList(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, unquoteTOML(p))
	}
	return out
}

func parseIntList(s string) []int64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := splitTOMLList(s)
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func splitTOMLList(s string) []string {
	var parts []string
	var b strings.Builder
	inSQ, inDQ := false, false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inDQ {
			inSQ = !inSQ
		} else if c == '"' && !inSQ {
			inDQ = !inDQ
		} else if !inSQ && !inDQ {
			switch c {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			case ',':
				if depth == 0 {
					parts = append(parts, b.String())
					b.Reset()
					continue
				}
			}
		}
		b.WriteByte(c)
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func tomlishToJSON(s string) string {
	// best-effort: single quotes -> double quotes
	return strings.ReplaceAll(s, "'", `"`)
}

func extractExamples(help, defaultRaw string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, line := range strings.Split(help, "\n") {
		line = strings.TrimSpace(line)
		if eq := strings.Index(line, " = "); eq > 0 {
			rhs := strings.TrimSpace(line[eq+3:])
			if rhs != "" && (strings.ContainsAny(rhs, `"'[]`) || strings.Contains(rhs, "=") == false) {
				add(rhs)
			}
		}
		// backtick snippets
		for {
			i := strings.Index(line, "`")
			if i < 0 {
				break
			}
			rest := line[i+1:]
			j := strings.Index(rest, "`")
			if j < 0 {
				break
			}
			add(rest[:j])
			line = rest[j+1:]
		}
	}
	if strings.TrimSpace(defaultRaw) != "" {
		add(strings.TrimSpace(defaultRaw))
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func hasWarning(help string) bool {
	u := strings.ToUpper(help)
	return strings.Contains(u, "WARNING") || strings.Contains(u, "!!!") || strings.Contains(u, "DO NOT ENABLE")
}

func nonEmpty(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitKeepNL(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.SplitAfter(s, "\n")
}

func stripNL(s string) string {
	return strings.TrimRight(s, "\r\n")
}
