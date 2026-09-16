package detect

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCLIVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"2.1.18", "2.1.18"},
		{"dnscrypt-proxy 2.1.18", "2.1.18"},
		{"dnscrypt-proxy version 2.1.5", "2.1.5"},
		{"v2.1.18", "2.1.18"},
		{"dnscrypt-proxy 2.1.18-beta.1\nextra", "2.1.18-beta.1"},
		{"DNSCrypt-Proxy 2.1.18 (Windows)", "2.1.18"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			v, err := ParseCLIVersion(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if v.String() != c.want {
				t.Fatalf("got %q want %q", v.String(), c.want)
			}
		})
	}
	if _, err := ParseCLIVersion(""); err == nil {
		t.Fatal("empty should fail")
	}
	if _, err := ParseCLIVersion("not a version at all"); err == nil {
		t.Fatal("garbage should fail")
	}
}

func TestParseSCBinaryPath(t *testing.T) {
	t.Parallel()
	out := `
[SC] QueryServiceConfig SUCCESS

SERVICE_NAME: dnscrypt-proxy
        BINARY_PATH_NAME   : "C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe" -service
        DISPLAY_NAME       : dnscrypt-proxy
`
	p, ok := ParseSCBinaryPath(out)
	if !ok {
		t.Fatal("expected path")
	}
	if p != `C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe` {
		t.Fatalf("got %q", p)
	}
	if _, ok := ParseSCBinaryPath("nope"); ok {
		t.Fatal("expected miss")
	}
}

func TestParseSystemctlExecStart(t *testing.T) {
	t.Parallel()
	in := `ExecStart={ path=/opt/dnscrypt-proxy/dnscrypt-proxy ; argv[]=/opt/dnscrypt-proxy/dnscrypt-proxy -config /opt/dnscrypt-proxy/dnscrypt-proxy.toml ; }`
	p, ok := ParseSystemctlExecStart(in)
	if !ok || p != "/opt/dnscrypt-proxy/dnscrypt-proxy" {
		t.Fatalf("got %q ok=%v", p, ok)
	}
}

func TestDetectOverrideWins(t *testing.T) {
	t.Parallel()
	r := Runner{
		GOOS: "linux",
		LookPath: func(string) (string, error) {
			return "/opt/dnscrypt-proxy/dnscrypt-proxy", nil
		},
		IsExecutable: func(string) bool { return true },
		RunVersion: func(context.Context, string) (string, error) {
			return "dnscrypt-proxy 2.1.14", nil
		},
	}
	res := r.Detect(context.Background(), "", "v2.1.10")
	if !res.Found || res.Source != SourceOverride || res.Version.String() != "2.1.10" {
		t.Fatalf("%+v", res)
	}
	if res.Path != "/opt/dnscrypt-proxy/dnscrypt-proxy" {
		t.Fatalf("path = %q", res.Path)
	}
}

func TestDetectBinaryFromPATH(t *testing.T) {
	t.Parallel()
	r := Runner{
		GOOS: "linux",
		LookPath: func(file string) (string, error) {
			if file != "dnscrypt-proxy" {
				t.Fatalf("LookPath(%q)", file)
			}
			return "/usr/bin/dnscrypt-proxy", nil
		},
		IsExecutable: func(string) bool { return true },
		RunVersion: func(_ context.Context, path string) (string, error) {
			if path != "/usr/bin/dnscrypt-proxy" {
				t.Fatalf("path %s", path)
			}
			return "2.1.18", nil
		},
	}
	res := r.Detect(context.Background(), "", "")
	if !res.Found || res.Source != SourceBinary || res.Version.String() != "2.1.18" {
		t.Fatalf("%+v", res)
	}
}

func TestDetectConfiguredPath(t *testing.T) {
	t.Parallel()
	want := filepath.FromSlash("/opt/dnscrypt-proxy/dnscrypt-proxy")
	r := Runner{
		GOOS: "linux",
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not run when binary_path is set")
			return "", errors.New("no")
		},
		IsExecutable: func(p string) bool { return p == want },
		RunVersion: func(context.Context, string) (string, error) {
			return "2.0.0", nil
		},
	}
	res := r.Detect(context.Background(), want, "")
	if !res.Found || res.Path != want || res.Version.String() != "2.0.0" {
		t.Fatalf("%+v", res)
	}
}

func TestDetectNotFound(t *testing.T) {
	t.Parallel()
	r := Runner{
		GOOS:         "linux",
		LookPath:     func(string) (string, error) { return "", errors.New("not in path") },
		IsExecutable: func(string) bool { return false },
	}
	res := r.Detect(context.Background(), "", "")
	if res.Found {
		t.Fatalf("found: %+v", res)
	}
	if res.Source != SourceNone {
		t.Fatalf("source %s", res.Source)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "not found") {
		t.Fatalf("err = %v", res.Err)
	}
}

func TestDetectInvalidOverride(t *testing.T) {
	t.Parallel()
	r := Runner{GOOS: "linux", LookPath: func(string) (string, error) { return "", errors.New("x") }, IsExecutable: func(string) bool { return false }}
	res := r.Detect(context.Background(), "", "not-a-version")
	if res.Found || res.Err == nil {
		t.Fatalf("%+v", res)
	}
}

func TestWindowsBinaryNameAndCommonPaths(t *testing.T) {
	t.Parallel()
	r := Runner{
		GOOS: "windows",
		Getenv: func(k string) string {
			if k == "ProgramFiles" {
				return `C:\Program Files`
			}
			return ""
		},
		UserHomeDir: func() (string, error) { return `C:\Users\sam`, nil },
	}
	if r.binaryName() != "dnscrypt-proxy.exe" {
		t.Fatal(r.binaryName())
	}
	paths := r.windowsCommonPaths()
	joined := strings.Join(paths, "\n")
	wantPF := filepath.Join(`C:\Program Files`, "dnscrypt-proxy", "dnscrypt-proxy.exe")
	if !strings.Contains(joined, wantPF) {
		t.Fatalf("missing PF path %q in %s", wantPF, joined)
	}
	if !strings.Contains(joined, filepath.Join("scoop", "apps", "dnscrypt-proxy", "current", "dnscrypt-proxy.exe")) {
		t.Fatalf("missing scoop path: %s", joined)
	}
}

func TestUnixWikiPathIncluded(t *testing.T) {
	t.Parallel()
	r := Runner{GOOS: "linux"}
	found := false
	for _, p := range r.unixCommonPaths() {
		if p == "/opt/dnscrypt-proxy/dnscrypt-proxy" {
			found = true
		}
	}
	if !found {
		t.Fatal("wiki path /opt/dnscrypt-proxy/dnscrypt-proxy missing")
	}
}
