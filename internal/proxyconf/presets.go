package proxyconf

import "encoding/json"

// Preset is a named overlay of toml patches (not a full rewrite).
type Preset struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Patches     []Patch `json:"patches"`
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func setBool(path string, v bool) Patch {
	return Patch{Path: path, Enabled: true, Value: mustJSON(v)}
}

func setString(path, v string) Patch {
	return Patch{Path: path, Enabled: true, Value: mustJSON(v)}
}

func setList(path string, v []string) Patch {
	return Patch{Path: path, Enabled: true, Value: mustJSON(v)}
}

func setAny(path string, v any) Patch {
	return Patch{Path: path, Enabled: true, Value: mustJSON(v)}
}

func disable(path string) Patch {
	return Patch{Path: path, Enabled: false}
}

// Presets returns curated overlays. Paths are catalog keys (validated in tests).
func Presets() []Preset {
	return []Preset{
		{
			ID:          "balanced",
			Name:        "Balanced",
			Description: "Upstream example defaults: no-log, no-filter resolvers, local listen, cache on.",
			Patches: []Patch{
				setList("listen_addresses", []string{"127.0.0.1:53"}),
				disable("server_names"),
				setBool("ipv4_servers", true),
				setBool("ipv6_servers", false),
				setBool("dnscrypt_servers", true),
				setBool("doh_servers", true),
				setBool("odoh_servers", false),
				setBool("require_dnssec", false),
				setBool("require_nolog", true),
				setBool("require_nofilter", true),
				setBool("force_tcp", false),
				setBool("cache", true),
				setBool("block_ipv6", false),
			},
		},
		{
			ID:          "privacy",
			Name:        "Privacy",
			Description: "Prefer no-log unfiltered resolvers, disable query logging, keep the monitoring UI off.",
			Patches: []Patch{
				setBool("require_nolog", true),
				setBool("require_nofilter", true),
				setBool("require_dnssec", true),
				disable("query_log.file"),
				setBool("monitoring_ui.enabled", false),
				setBool("tls_disable_session_tickets", true),
			},
		},
		{
			ID:          "family",
			Name:        "Family / filtered",
			Description: "Allow resolvers that enforce their own blocklists (parental control, ads).",
			Patches: []Patch{
				setBool("require_nofilter", false),
				setBool("require_nolog", true),
			},
		},
		{
			ID:          "anonymized",
			Name:        "Anonymized DNS",
			Description: "Route DNSCrypt via relays. Review relays.md and replace the catch-all route when you can.",
			Patches: []Patch{
				setBool("anonymized_dns.skip_incompatible", true),
				setAny("anonymized_dns.routes", []map[string]any{
					{"server_name": "*", "via": []string{"*"}},
				}),
			},
		},
		{
			ID:          "tor",
			Name:        "Tor",
			Description: "Force TCP and send it through a local Tor SOCKS proxy (Tor must already be running).",
			Patches: []Patch{
				setBool("force_tcp", true),
				setString("proxy", "socks5://dnscrypt:dnscrypt@127.0.0.1:9050"),
			},
		},
		{
			ID:          "performance",
			Name:        "Performance",
			Description: "Keep the cache large and use weighted load balancing.",
			Patches: []Patch{
				setBool("cache", true),
				setAny("cache_size", 4096),
				setString("lb_strategy", "wp2"),
				setBool("lb_estimator", true),
			},
		},
		{
			ID:          "dnssec",
			Name:        "Strict DNSSEC",
			Description: "Only use resolvers that support DNSSEC.",
			Patches: []Patch{
				setBool("require_dnssec", true),
			},
		},
	}
}

// ValidatePresets reports preset paths missing from the catalog.
func ValidatePresets(cat Catalog) error {
	for _, p := range Presets() {
		for _, patch := range p.Patches {
			if _, ok := cat.FieldByPath(patch.Path); !ok {
				return errMissingPresetPath(p.ID, patch.Path)
			}
		}
	}
	return nil
}

type missingPathError struct {
	Preset string
	Path   string
}

func errMissingPresetPath(preset, path string) error {
	return missingPathError{Preset: preset, Path: path}
}

func (e missingPathError) Error() string {
	return "preset " + e.Preset + " references unknown catalog path " + e.Path
}
