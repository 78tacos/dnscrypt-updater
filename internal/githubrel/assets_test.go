package githubrel

import "testing"

func TestPlatformToken(t *testing.T) {
	t.Parallel()
	cases := []struct{ goos, arch, want string }{
		{"windows", "amd64", "win64"},
		{"windows", "386", "win32"},
		{"windows", "arm64", "winarm"},
		{"linux", "amd64", "linux_x86_64"},
		{"linux", "386", "linux_i386"},
		{"linux", "arm64", "linux_arm64"},
		{"linux", "arm", "linux_arm"},
		{"darwin", "amd64", "macos_x86_64"},
		{"darwin", "arm64", "macos_arm64"},
		{"freebsd", "amd64", "freebsd_amd64"},
		{"plan9", "amd64", ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.goos+"/"+c.arch, func(t *testing.T) {
			t.Parallel()
			if got := PlatformToken(c.goos, c.arch); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestSelectSignedArchivePairsMinisig(t *testing.T) {
	t.Parallel()
	assets := []Asset{
		{Name: "dnscrypt-proxy-win64-2.1.18.zip", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip"},
		{Name: "dnscrypt-proxy-win64-2.1.18.zip.minisig", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip.minisig"},
		{Name: "dnscrypt-proxy-win32-2.1.18.zip", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win32-2.1.18.zip"},
		{Name: "dnscrypt-proxy-win32-2.1.18.zip.minisig", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win32-2.1.18.zip.minisig"},
		{Name: "dnscrypt-proxy-x64-2.1.18.msi", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-x64-2.1.18.msi"},
		{Name: "dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz"},
		{Name: "dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz.minisig", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz.minisig"},
		{Name: "dnscrypt-proxy-linux_arm64-2.1.18.tar.gz", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-linux_arm64-2.1.18.tar.gz"},
		{Name: "dnscrypt-proxy-linux_arm64-2.1.18.tar.gz.minisig", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-linux_arm64-2.1.18.tar.gz.minisig"},
	}
	win, ok := SelectSignedArchive("windows", "amd64", assets)
	if !ok || win.Archive.Name != "dnscrypt-proxy-win64-2.1.18.zip" {
		t.Fatalf("win64: %+v ok=%v", win, ok)
	}
	if win.Minisig.Name != "dnscrypt-proxy-win64-2.1.18.zip.minisig" {
		t.Fatalf("minisig %q", win.Minisig.Name)
	}
	if got, ok := SelectSignedArchive("linux", "amd64", assets); !ok || got.Archive.Name != "dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz" {
		t.Fatalf("linux amd64: %+v ok=%v", got, ok)
	}
	if _, ok := SelectSignedArchive("linux", "arm", assets); ok {
		t.Fatal("linux_arm must not match linux_arm64")
	}
	if _, ok := SelectSignedArchive("windows", "amd64", []Asset{{Name: "dnscrypt-proxy-x64-2.1.18.msi"}}); ok {
		t.Fatal("unsigned MSI must not be selected")
	}
}

func TestMinisignPubKeyMatchesScout(t *testing.T) {
	t.Parallel()
	const want = "RWTk1xXqcTODeYttYMCMLo0YJHaFEHn7a3akqHlb/7QvIQXHVPxKbjB5"
	if MinisignPubKey != want {
		t.Fatalf("pubkey %q", MinisignPubKey)
	}
}
