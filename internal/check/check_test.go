package check

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/detect"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
	"github.com/78tacos/dnscrypt-updater/internal/version"
)

type ghStub struct {
	res githubrel.Result
	err error
	saw string
}

func (g *ghStub) Latest(_ context.Context, etag string) (githubrel.Result, error) {
	g.saw = etag
	return g.res, g.err
}

type detStub struct {
	res detect.Result
}

func (d detStub) Detect(context.Context, string, string) detect.Result { return d.res }

func ver(t *testing.T, s string) version.Version {
	t.Helper()
	v, err := version.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestUpdateAvailableNormalizesV(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	e := Engine{
		GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{
			TagName: "v2.1.18",
			HTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
		}}},
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.14"), Source: detect.SourceBinary, Path: "/x"}},
		Now:    func() time.Time { return now },
	}
	res, st, err := e.Run(context.Background(), config.File{Notify: true}, config.State{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpdateAvailable || !res.ShouldNotify {
		t.Fatalf("%+v", res)
	}
	if res.RemoteVersion != "2.1.18" || res.LocalVersion != "2.1.14" {
		t.Fatalf("%+v", res)
	}
	if st.LastNotifiedVersion != "" {
		t.Fatalf("engine must not mark notified until a notification is sent: %+v", st)
	}
}

func TestNoNotifyWhenCurrent(t *testing.T) {
	t.Parallel()
	e := Engine{
		GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{TagName: "2.1.18", HTMLURL: "https://example"}}},
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "v2.1.18"), Source: detect.SourceOverride}},
		Now:    time.Now,
	}
	res, _, err := e.Run(context.Background(), config.File{Notify: true}, config.State{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.UpdateAvailable || res.ShouldNotify {
		t.Fatalf("%+v", res)
	}
}

func TestSkipAndSnoozeAndAlreadyNotified(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	baseGH := &ghStub{res: githubrel.Result{Release: githubrel.Release{TagName: "2.1.18", HTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18"}}}
	det := detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.14"), Source: detect.SourceBinary}}
	e := Engine{GitHub: baseGH, Detect: det, Now: func() time.Time { return now }}

	res, _, err := e.Run(context.Background(), config.File{Notify: true, SkipVersion: "2.1.18"}, config.State{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpdateAvailable || !res.Skipped || res.ShouldNotify {
		t.Fatalf("skip: %+v", res)
	}

	res, _, err = e.Run(context.Background(), config.File{Notify: true, SnoozeUntil: now.Add(time.Hour).Format(time.RFC3339)}, config.State{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Snoozed || res.ShouldNotify {
		t.Fatalf("snooze: %+v", res)
	}

	res, _, err = e.Run(context.Background(), config.File{Notify: true, SnoozeUntil: now.Add(time.Hour).Format(time.RFC3339)}, config.State{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.ShouldNotify {
		t.Fatalf("force during snooze: %+v", res)
	}

	res, _, err = e.Run(context.Background(), config.File{Notify: true}, config.State{LastNotifiedVersion: "2.1.18"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ShouldNotify {
		t.Fatalf("already notified: %+v", res)
	}
	res, _, err = e.Run(context.Background(), config.File{Notify: true}, config.State{LastNotifiedVersion: "2.1.18"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.ShouldNotify {
		t.Fatalf("force re-notify: %+v", res)
	}
}

func TestNotModifiedUsesCache(t *testing.T) {
	t.Parallel()
	gh := &ghStub{res: githubrel.Result{NotModified: true, ETag: `W/"x"`}}
	e := Engine{
		GitHub: gh,
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.14"), Source: detect.SourceBinary}},
		Now:    time.Now,
	}
	st := config.State{ETag: `W/"x"`, CachedTag: "2.1.18", CachedHTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18"}
	res, _, err := e.Run(context.Background(), config.File{Notify: true}, st, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotModified || res.RemoteVersion != "2.1.18" || !res.UpdateAvailable {
		t.Fatalf("%+v", res)
	}
	if gh.saw != `W/"x"` {
		t.Fatalf("etag sent %q", gh.saw)
	}
}

func TestNotFoundDoesNotInventVersion(t *testing.T) {
	t.Parallel()
	e := Engine{
		GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{TagName: "2.1.18", HTMLURL: "https://x"}}},
		Detect: detStub{res: detect.Result{Found: false, Source: detect.SourceNone, Err: errors.New("dnscrypt-proxy not found")}},
		Now:    time.Now,
	}
	res, _, err := e.Run(context.Background(), config.File{Notify: true}, config.State{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotFound || res.LocalVersion != "" || res.UpdateAvailable {
		t.Fatalf("%+v", res)
	}
	if !res.ShouldNotify {
		t.Fatalf("want not-found notify: %+v", res)
	}
}

func TestNotifyDisabled(t *testing.T) {
	t.Parallel()
	e := Engine{
		GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{TagName: "2.1.18", HTMLURL: "https://x"}}},
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.0"), Source: detect.SourceBinary}},
		Now:    time.Now,
	}
	res, _, err := e.Run(context.Background(), config.File{Notify: false}, config.State{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpdateAvailable || res.ShouldNotify {
		t.Fatalf("%+v", res)
	}
}

func TestOfficialSignedAssetSelected(t *testing.T) {
	t.Parallel()
	e := Engine{
		GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{
			TagName: "2.1.18",
			HTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
			Assets: []githubrel.Asset{
				{Name: "dnscrypt-proxy-win64-2.1.18.zip", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip"},
				{Name: "dnscrypt-proxy-win64-2.1.18.zip.minisig", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-win64-2.1.18.zip.minisig"},
				{Name: "dnscrypt-proxy-x64-2.1.18.msi", BrowserDownloadURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/download/2.1.18/dnscrypt-proxy-x64-2.1.18.msi"},
			},
		}}},
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.14"), Source: detect.SourceBinary}},
		Now:    time.Now,
		GOOS:   "windows",
		GOARCH: "amd64",
	}
	res, st, err := e.Run(context.Background(), config.File{Notify: true}, config.State{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.OfficialAsset != "dnscrypt-proxy-win64-2.1.18.zip" {
		t.Fatalf("asset %q", res.OfficialAsset)
	}
	if res.MinisigName != "dnscrypt-proxy-win64-2.1.18.zip.minisig" {
		t.Fatalf("minisig %q", res.MinisigName)
	}
	if res.MinisignPubKey != githubrel.MinisignPubKey {
		t.Fatalf("pubkey %q", res.MinisignPubKey)
	}
	if st.CachedAssetName != res.OfficialAsset {
		t.Fatalf("cache %q", st.CachedAssetName)
	}
}

func TestNotModifiedRestoresCachedAsset(t *testing.T) {
	t.Parallel()
	e := Engine{
		GitHub: &ghStub{res: githubrel.Result{NotModified: true}},
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.14"), Source: detect.SourceBinary}},
		Now:    time.Now,
		GOOS:   "linux",
		GOARCH: "amd64",
	}
	st := config.State{
		CachedTag:         "2.1.18",
		CachedHTMLURL:     "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
		CachedAssetName:   "dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz",
		CachedMinisigName: "dnscrypt-proxy-linux_x86_64-2.1.18.tar.gz.minisig",
	}
	res, _, err := e.Run(context.Background(), config.File{Notify: true}, st, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotModified || res.OfficialAsset != st.CachedAssetName {
		t.Fatalf("%+v", res)
	}
}

func TestGitHubError(t *testing.T) {
	t.Parallel()
	e := Engine{
		GitHub: &ghStub{err: errors.New("boom")},
		Detect: detStub{res: detect.Result{Found: true, Version: ver(t, "2.1.0"), Source: detect.SourceBinary}},
		Now:    time.Now,
	}
	res, _, err := e.Run(context.Background(), config.File{Notify: true}, config.State{CachedTag: "2.1.17"}, false)
	if err == nil {
		t.Fatal("want err")
	}
	if res.RemoteVersion != "2.1.17" {
		t.Fatalf("cache fallback %+v", res)
	}
}
