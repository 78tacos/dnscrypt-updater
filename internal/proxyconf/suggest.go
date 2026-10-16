package proxyconf

import (
	"net"
	"strings"
)

// Suggestion is a dismissible hint, never auto-applied.
type Suggestion struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Path   string `json:"path,omitempty"`
	Level  string `json:"level"` // warning or info
}

func presentValue(cur map[string]Value, path string) (Value, bool) {
	v, ok := cur[path]
	if !ok || !v.Present {
		return Value{}, false
	}
	return v, true
}

func asBool(v Value) (bool, bool) {
	b, ok := v.Decoded.(bool)
	return b, ok
}

func asString(v Value) string {
	if s, ok := v.Decoded.(string); ok {
		return s
	}
	return strings.Trim(v.Raw, `"' `)
}

func asList(v Value) []string {
	switch t := v.Decoded.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// Suggestions builds WARNING-comment hints plus a few heuristics.
func Suggestions(cat Catalog, cur map[string]Value) []Suggestion {
	var out []Suggestion

	if v, ok := presentValue(cur, "ipv6_servers"); ok {
		if b, ok := asBool(v); ok && b {
			out = append(out, Suggestion{
				ID:     "ipv6_servers",
				Title:  "IPv6 resolvers are enabled",
				Detail: "Leave this off unless this machine has working IPv6. Broken IPv6 makes lookups slow or flaky.",
				Path:   "ipv6_servers",
				Level:  "warning",
			})
		}
	}
	if v, ok := presentValue(cur, "http3_probe"); ok {
		if b, ok := asBool(v); ok && b {
			out = append(out, Suggestion{
				ID:     "http3_probe",
				Title:  "HTTP/3 probe is on",
				Detail: "Upstream warns this can make DoH much slower for servers that do not advertise HTTP/3.",
				Path:   "http3_probe",
				Level:  "warning",
			})
		}
	}
	monOn := false
	if v, ok := presentValue(cur, "monitoring_ui.enabled"); ok {
		if b, ok := asBool(v); ok && b {
			monOn = true
		}
	}
	if monOn {
		if v, ok := presentValue(cur, "monitoring_ui.listen_address"); ok {
			addr := asString(v)
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			if host != "127.0.0.1" && host != "::1" && host != "localhost" {
				out = append(out, Suggestion{
					ID:     "monitoring_bind",
					Title:  "Monitoring UI is not loopback-only",
					Detail: "Bind 127.0.0.1 unless you intend to expose query stats on the network.",
					Path:   "monitoring_ui.listen_address",
					Level:  "warning",
				})
			}
		}
		if v, ok := presentValue(cur, "monitoring_ui.password"); ok {
			if asString(v) == "changeme" {
				out = append(out, Suggestion{
					ID:     "monitoring_password",
					Title:  "Monitoring UI still uses the example password",
					Detail: "Change the password (or disable the UI) before enabling it.",
					Path:   "monitoring_ui.password",
					Level:  "warning",
				})
			}
		}
	}
	if v, ok := presentValue(cur, "server_names"); ok {
		if len(asList(v)) > 0 {
			out = append(out, Suggestion{
				ID:     "server_names",
				Title:  "server_names is set",
				Detail: "require_nolog, require_nofilter, and require_dnssec do not apply to this explicit list.",
				Path:   "server_names",
				Level:  "info",
			})
		}
	}

	for _, f := range cat.Fields {
		if !f.Warning {
			continue
		}
		v, ok := presentValue(cur, f.Path)
		if !ok {
			continue
		}
		if b, isBool := asBool(v); isBool && !b {
			continue
		}
		id := "warn:" + f.Path
		dup := false
		for _, s := range out {
			if s.Path == f.Path {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		out = append(out, Suggestion{
			ID:     id,
			Title:  f.Path + " has an upstream warning",
			Detail: firstLine(f.Help),
			Path:   f.Path,
			Level:  "warning",
		})
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}
