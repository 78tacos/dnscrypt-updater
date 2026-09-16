package openurl

import "testing"

func TestValidateReleaseURL(t *testing.T) {
	t.Parallel()
	ok, err := ValidateReleaseURL("https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18")
	if err != nil {
		t.Fatal(err)
	}
	if ok == "" {
		t.Fatal("empty")
	}
	if _, err := ValidateReleaseURL("https://github.com/DNSCrypt/dnscrypt-proxy/releases"); err != nil {
		t.Fatal(err)
	}

	bads := []string{
		"",
		"http://github.com/DNSCrypt/dnscrypt-proxy/releases",
		"https://evil.example/releases",
		"https://github.com/78tacos/dnscrypt-proxy/releases",
		"https://github.com/DNSCrypt/other/releases",
	}
	for _, u := range bads {
		if _, err := ValidateReleaseURL(u); err == nil {
			t.Fatalf("expected reject %q", u)
		}
	}
}
