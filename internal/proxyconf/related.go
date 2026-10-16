package proxyconf

// Companion list files the UI may create or edit. Auto-downloaded
// public-resolvers.md / relays.md caches are intentionally omitted.
var companionFiles = []struct {
	Path    string
	Live    string
	Example string
}{
	{Path: "forwarding_rules", Live: "forwarding-rules.txt", Example: "example-forwarding-rules.txt"},
	{Path: "cloaking_rules", Live: "cloaking-rules.txt", Example: "example-cloaking-rules.txt"},
	{Path: "blocked_names.blocked_names_file", Live: "blocked-names.txt", Example: "example-blocked-names.txt"},
	{Path: "blocked_ips.blocked_ips_file", Live: "blocked-ips.txt", Example: "example-blocked-ips.txt"},
	{Path: "allowed_names.allowed_names_file", Live: "allowed-names.txt", Example: "example-allowed-names.txt"},
	{Path: "allowed_ips.allowed_ips_file", Live: "allowed-ips.txt", Example: "example-allowed-ips.txt"},
	{Path: "captive_portals.map_file", Live: "captive-portals.txt", Example: "example-captive-portals.txt"},
}

func relatedByPath() map[string][2]string {
	m := make(map[string][2]string, len(companionFiles))
	for _, c := range companionFiles {
		m[c.Path] = [2]string{c.Live, c.Example}
	}
	return m
}

func companionLiveNames() []string {
	out := make([]string, 0, len(companionFiles))
	for _, c := range companionFiles {
		out = append(out, c.Live)
	}
	return out
}

// easyPaths are shown on the Easy tab. Keys must exist in the generated catalog.
var easyPaths = []string{
	"listen_addresses",
	"server_names",
	"disabled_server_names",
	"ipv4_servers",
	"ipv6_servers",
	"dnscrypt_servers",
	"doh_servers",
	"odoh_servers",
	"require_dnssec",
	"require_nolog",
	"require_nofilter",
	"force_tcp",
	"http3",
	"proxy",
	"timeout",
	"bootstrap_resolvers",
	"ignore_system_dns",
	"block_ipv6",
	"cache",
	"cache_size",
	"lb_strategy",
	"forwarding_rules",
	"cloaking_rules",
	"blocked_names.blocked_names_file",
	"allowed_names.allowed_names_file",
	"query_log.file",
	"anonymized_dns.skip_incompatible",
	"anonymized_dns.routes",
	"monitoring_ui.enabled",
	"monitoring_ui.listen_address",
	"monitoring_ui.username",
	"monitoring_ui.password",
	"pqdnscrypt",
}
