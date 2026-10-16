package apply

import (
	"strings"
	"testing"
)

func TestWindowsCmdlineArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"-quiet", "-quiet"},
		{`C:\Users\a\AppData\Local\Temp\x`, `C:\Users\a\AppData\Local\Temp\x`},
		{`C:\Program Files\dnscrypt-proxy`, `"C:\Program Files\dnscrypt-proxy"`},
		{`C:\Users\John Doe\AppData\Local\Temp\s`, `"C:\Users\John Doe\AppData\Local\Temp\s"`},
		{`say "hi"`, `"say ""hi"""`},
		{"", `""`},
	}
	for _, tc := range cases {
		if got := windowsCmdlineArg(tc.in); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestPowershellArgumentListPreservesSpacedPaths(t *testing.T) {
	t.Parallel()
	got := powershellArgumentList([]string{
		"-apply-config",
		"-quiet",
		"-config", `C:\Users\Jane Doe\AppData\Roaming\dnscrypt-proxy-updater\config.json`,
		"-staging", `C:\Users\Jane Doe\AppData\Local\Temp\dnscrypt-settings-1`,
		"-install-dir", `C:\Program Files\dnscrypt-proxy`,
		"-binary-path", `C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe`,
	})
	needles := []string{
		`'"C:\Users\Jane Doe\AppData\Roaming\dnscrypt-proxy-updater\config.json"'`,
		`'"C:\Users\Jane Doe\AppData\Local\Temp\dnscrypt-settings-1"'`,
		`'"C:\Program Files\dnscrypt-proxy"'`,
		`'"C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe"'`,
		`'-apply-config'`,
		`'-quiet'`,
	}
	for _, n := range needles {
		if !strings.Contains(got, n) {
			t.Fatalf("missing %s in %s", n, got)
		}
	}
}
