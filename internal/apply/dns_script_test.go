package apply

import (
	"strings"
	"testing"
)

func TestWindowsDNSScriptSetsIPv4AndGuardedIPv6(t *testing.T) {
	t.Parallel()
	script := windowsDNSScript
	if !strings.Contains(script, "127.0.0.1") {
		t.Fatal("script must set IPv4 DNS to 127.0.0.1")
	}
	bind := strings.Index(script, "ms_tcpip6")
	v6 := strings.Index(script, "::1")
	if bind < 0 || !strings.Contains(script, "Get-NetAdapterBinding") {
		t.Fatal("script must check the ms_tcpip6 adapter binding before setting IPv6 DNS")
	}
	if v6 < 0 || v6 < bind {
		t.Fatal("::1 must be applied only after the IPv6 binding check")
	}
	if !strings.Contains(script, "catch") || !strings.Contains(script, "skip6:") {
		t.Fatal("adapters without IPv6 must be skipped without failing the script")
	}
	if strings.Contains(script, "ResetServerAddresses") {
		t.Fatal("script must not reset DNS server addresses")
	}
	// IPv4 is set on its own so a missing IPv6 stack cannot fail that update.
	if !strings.Contains(script, "ServerAddresses @('127.0.0.1')") {
		t.Fatal("IPv4 loopback must be set in its own call")
	}
	if strings.Contains(script, "127.0.0.1', '::1") || strings.Contains(script, "127.0.0.1','::1") {
		t.Fatal("do not pass both families in one call; IPv6-disabled adapters fail the whole cmdlet")
	}
}
