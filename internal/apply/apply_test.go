package apply

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
)

func TestValidateAssetURL(t *testing.T) {
	t.Parallel()
	ok := []string{
		"https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip",
		"https://objects.githubusercontent.com/foo",
		"https://release-assets.githubusercontent.com/foo",
	}
	for _, u := range ok {
		if err := ValidateAssetURL(u); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
	bads := []string{
		"http://github.com/DNSCrypt/dnscrypt-proxy/releases/download/x.zip",
		"https://evil.example/x.zip",
		"https://github.com.evil.example/x.zip",
	}
	for _, u := range bads {
		if err := ValidateAssetURL(u); err == nil {
			t.Fatalf("expected reject %q", u)
		}
	}
}

func TestPackageManagedAndInstallDir(t *testing.T) {
	t.Parallel()
	if !packageManagedPath(`C:\Users\sam\scoop\apps\dnscrypt-proxy\current\dnscrypt-proxy.exe`) {
		t.Fatal("scoop")
	}
	if !packageManagedPath(`C:/Users/sam/scoop/apps/dnscrypt-proxy/current/dnscrypt-proxy.exe`) {
		t.Fatal("scoop slash")
	}
	if packageManagedPath(`C:\Program Files\dnscrypt-proxy\dnscrypt-proxy.exe`) {
		t.Fatal("program files is not package-managed")
	}
	got := resolveInstallDir("", filepath.Join("C:", "Program Files", "dnscrypt-proxy", "dnscrypt-proxy.exe"), "windows", func(string) string { return filepath.Join("C:", "Program Files") })
	if got != filepath.Join("C:", "Program Files", "dnscrypt-proxy") {
		t.Fatalf("in-place dir %q", got)
	}
	got = resolveInstallDir("", `C:\Users\sam\scoop\apps\dnscrypt-proxy\current\dnscrypt-proxy.exe`, "windows", func(k string) string {
		if k == "ProgramFiles" {
			return filepath.Join("C:", "Program Files")
		}
		return ""
	})
	want := filepath.Join("C:", "Program Files", "dnscrypt-proxy")
	if got != want {
		t.Fatalf("scoop should install to PF, got %q", got)
	}
	if DefaultInstallDir("linux", nil) != "/opt/dnscrypt-proxy" {
		t.Fatal(DefaultInstallDir("linux", nil))
	}
}

func TestExtractZipAndZipSlip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ok.zip")
	if err := writeZip(zipPath, map[string]string{
		"dnscrypt-proxy.exe":          "binary",
		"example-dnscrypt-proxy.toml": "listen_addresses = ['127.0.0.1:53']\n",
		"LICENSE":                     "MIT",
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if err := extractArchive(zipPath, dest); err != nil {
		t.Fatal(err)
	}
	bin, err := findProxyBinary(dest, "windows")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(bin)
	if string(b) != "binary" {
		t.Fatalf("bin %q", b)
	}

	slip := filepath.Join(dir, "slip.zip")
	if err := writeZip(slip, map[string]string{"../../evil.txt": "nope"}); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(slip, filepath.Join(dir, "safe")); err == nil {
		t.Fatal("expected zip slip reject")
	}
}

func TestExtractTarGz(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "p.tar.gz")
	if err := writeTarGz(tarPath, map[string]string{
		"dnscrypt-proxy": "unixbin",
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if err := extractArchive(tarPath, dest); err != nil {
		t.Fatal(err)
	}
	bin, err := findProxyBinary(dest, "linux")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(bin)
	if string(b) != "unixbin" {
		t.Fatalf("got %q", b)
	}
}

func TestInstallPayloadPreservesToml(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dnscrypt-proxy"), []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "example-dnscrypt-proxy.toml"), []byte("example"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "dnscrypt-proxy"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "dnscrypt-proxy.toml"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin, backup, fresh, err := installPayload(src, dest, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if fresh {
		t.Fatal("expected update")
	}
	got, _ := os.ReadFile(bin)
	if string(got) != "new" {
		t.Fatalf("binary %q", got)
	}
	old, _ := os.ReadFile(backup)
	if string(old) != "old" {
		t.Fatalf("backup %q", old)
	}
	toml, _ := os.ReadFile(filepath.Join(dest, "dnscrypt-proxy.toml"))
	if string(toml) != "mine" {
		t.Fatalf("toml overwritten: %q", toml)
	}
}

func TestInstallPayloadFreshCopiesExampleToml(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dnscrypt-proxy.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "example-dnscrypt-proxy.toml"), []byte("listen"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest")
	_, _, fresh, err := installPayload(src, dest, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		t.Fatal("expected fresh")
	}
	toml, err := os.ReadFile(filepath.Join(dest, "dnscrypt-proxy.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(toml) != "listen" {
		t.Fatalf("toml %q", toml)
	}
}

func TestVerifyArchive(t *testing.T) {
	t.Parallel()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "a.zip")
	if err := os.WriteFile(path, []byte("official-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r := minisign.NewReader(f)
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	sig := r.Sign(priv)
	pubText, err := pub.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive(path, sig, string(pubText)); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive(path, sig, githubrel.MinisignPubKey); err == nil {
		t.Fatal("wrong key must fail")
	}
	if err := verifyArchive(path, []byte("untrusted comment: x\nnot-a-sig\n"), string(pubText)); err == nil {
		t.Fatal("garbage sig must fail")
	}
}

func TestOfficialPubKeyParses(t *testing.T) {
	t.Parallel()
	var pk minisign.PublicKey
	if err := pk.UnmarshalText([]byte(githubrel.MinisignPubKey)); err != nil {
		t.Fatal(err)
	}
}

func TestApplyEndToEnd(t *testing.T) {
	t.Parallel()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubText, err := pub.MarshalText()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	zipPath := filepath.Join(dir, "dnscrypt-proxy-win64-2.1.18.zip")
	if err := writeZip(zipPath, map[string]string{
		"dnscrypt-proxy.exe":          "PROXY",
		"example-dnscrypt-proxy.toml": "listen_addresses = ['127.0.0.1:53']\n",
	}); err != nil {
		t.Fatal(err)
	}
	zf, err := os.Open(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	r := minisign.NewReader(zf)
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatal(err)
	}
	_ = zf.Close()
	sig := r.Sign(priv)
	sigPath := zipPath + ".minisig"
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dnscrypt-proxy-win64-2.1.18.zip", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, zipPath)
	})
	mux.HandleFunc("/dnscrypt-proxy-win64-2.1.18.zip.minisig", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, sigPath)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	svc := &fakeService{}
	dns := &fakeDNS{}
	dest := filepath.Join(dir, "Program Files", "dnscrypt-proxy")
	a := &Applier{
		HTTP:           srv.Client(),
		GOOS:           "windows",
		GOARCH:         "amd64",
		Service:        svc,
		DNS:            dns,
		Elevated:       func() bool { return true },
		RunCheck:       func(context.Context, string, string) error { return nil },
		AllowDownload:  func(string) error { return nil },
		MinisignPubKey: string(pubText),
	}
	archive := githubrel.SignedArchive{
		Archive: githubrel.Asset{
			Name:               "dnscrypt-proxy-win64-2.1.18.zip",
			BrowserDownloadURL: srv.URL + "/dnscrypt-proxy-win64-2.1.18.zip",
		},
		Minisig: githubrel.Asset{
			Name:               "dnscrypt-proxy-win64-2.1.18.zip.minisig",
			BrowserDownloadURL: srv.URL + "/dnscrypt-proxy-win64-2.1.18.zip.minisig",
		},
	}
	res, err := a.Apply(context.Background(), archive, "2.1.18", Options{
		InstallDir:    dest,
		SetSystemDNS:  true,
		ManageService: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || !res.FreshInstall || !res.ServiceStarted || !res.DNSUpdated {
		t.Fatalf("%+v", res)
	}
	if res.Version != "2.1.18" {
		t.Fatalf("version %q", res.Version)
	}
	got, err := os.ReadFile(res.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PROXY" {
		t.Fatalf("bin %q", got)
	}
	toml, _ := os.ReadFile(filepath.Join(dest, "dnscrypt-proxy.toml"))
	if !strings.Contains(string(toml), "127.0.0.1") {
		t.Fatalf("toml %q", toml)
	}
	if !containsOps(svc.ops, "install", "start") {
		t.Fatalf("ops %v", svc.ops)
	}
	if dns.n != 1 {
		t.Fatalf("dns calls %d", dns.n)
	}
}

func TestApplyRejectsBadSignature(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "dnscrypt-proxy-win64-2.1.18.zip")
	if err := writeZip(zipPath, map[string]string{"dnscrypt-proxy.exe": "x"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/a.zip", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, zipPath) })
	mux.HandleFunc("/a.zip.minisig", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("untrusted comment: x\nnot-a-real-signature\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	a := &Applier{
		HTTP:          srv.Client(),
		GOOS:          "windows",
		AllowDownload: func(string) error { return nil },
		Service:       &fakeService{},
		DNS:           &fakeDNS{},
		RunCheck:      func(context.Context, string, string) error { return nil },
	}
	_, err := a.Apply(context.Background(), githubrel.SignedArchive{
		Archive: githubrel.Asset{Name: "a.zip", BrowserDownloadURL: srv.URL + "/a.zip"},
		Minisig: githubrel.Asset{Name: "a.zip.minisig", BrowserDownloadURL: srv.URL + "/a.zip.minisig"},
	}, "2.1.18", Options{InstallDir: filepath.Join(dir, "dest")})
	if err == nil || !strings.Contains(err.Error(), "minisign") {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadRejectsNonGitHub(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	t.Cleanup(srv.Close)
	a := &Applier{HTTP: srv.Client(), UserAgent: "dnscrypt-proxy-updater-test"}
	dest := filepath.Join(t.TempDir(), "x.zip")
	if err := a.download(context.Background(), srv.URL+"/x.zip", dest); err == nil {
		t.Fatal("httptest host must be rejected")
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()
	s := summarize(Result{
		Version:        "2.1.18",
		InstallDir:     `/opt/dnscrypt-proxy`,
		FreshInstall:   true,
		Verified:       true,
		ServiceStarted: true,
		DNSUpdated:     true,
	})
	for _, want := range []string{"Installed", "2.1.18", "minisign OK", "service running", "127.0.0.1"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %q", want, s)
		}
	}
}

func TestSafeJoin(t *testing.T) {
	t.Parallel()
	dest := t.TempDir()
	if _, err := safeJoin(dest, "../outside"); err == nil {
		t.Fatal("expected reject")
	}
	p, err := safeJoin(dest, "nested/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, dest) {
		t.Fatalf("%s", p)
	}
}

func containsOps(ops []string, want ...string) bool {
	have := map[string]bool{}
	for _, o := range ops {
		have[o] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}

type fakeService struct {
	installed bool
	running   bool
	ops       []string
}

func (f *fakeService) Query(context.Context) (ServiceStatus, error) {
	return ServiceStatus{Installed: f.installed, Running: f.running}, nil
}
func (f *fakeService) Install(context.Context, string) error {
	f.ops = append(f.ops, "install")
	f.installed = true
	return nil
}
func (f *fakeService) Stop(context.Context, string) error {
	f.ops = append(f.ops, "stop")
	f.running = false
	return nil
}
func (f *fakeService) Start(context.Context, string) error {
	f.ops = append(f.ops, "start")
	f.running = true
	return nil
}

type fakeDNS struct{ n int }

func (f *fakeDNS) SetLoopback(context.Context) error { f.n++; return nil }

func writeZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, body := range files {
		fw, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(fw, body); err != nil {
			return err
		}
	}
	return w.Close()
}

func writeTarGz(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.WriteString(tw, body); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}
