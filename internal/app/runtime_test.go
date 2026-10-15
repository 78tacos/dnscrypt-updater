package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/78tacos/dnscrypt-updater/internal/check"
	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/detect"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
	"github.com/78tacos/dnscrypt-updater/internal/version"
)

func TestInstallSetsSystemDNS(t *testing.T) {
	t.Parallel()
	on := true
	off := false
	cases := []struct {
		name  string
		cfg   config.File
		noDNS bool
		want  bool
	}{
		{name: "unset defaults on", cfg: config.File{}, want: true},
		{name: "explicit true", cfg: config.File{SetSystemDNS: &on}, want: true},
		{name: "explicit false", cfg: config.File{SetSystemDNS: &off}, want: false},
		{name: "cli opt-out", cfg: config.File{SetSystemDNS: &on}, noDNS: true, want: false},
		{name: "config false and cli opt-out", cfg: config.File{SetSystemDNS: &off}, noDNS: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := installSetsSystemDNS(tc.cfg, tc.noDNS); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCheckOnceJSONUpdateExit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	v, err := version.Parse("2.1.14")
	if err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{
		Opts: Options{CheckOnce: true, JSON: true},
		Paths: config.Paths{
			Dir:   dir,
			File:  filepath.Join(dir, "config.json"),
			State: filepath.Join(dir, "state.json"),
			Log:   filepath.Join(dir, "x.log"),
		},
		Engine: check.Engine{
			GitHub: &ghStub{res: githubrel.Result{Release: githubrel.Release{
				TagName: "2.1.18",
				HTMLURL: "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
			}}},
			Detect: detStub{res: detect.Result{Found: true, Version: v, Source: detect.SourceBinary, Path: "/opt/dnscrypt-proxy/dnscrypt-proxy"}},
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg: config.File{Notify: true},
	}
	var buf bytes.Buffer
	code, err := rt.CheckOnce(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	var res check.Result
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatal(err, buf.String())
	}
	if !res.UpdateAvailable || res.RemoteVersion != "2.1.18" {
		t.Fatalf("%+v", res)
	}
}

type ghStub struct {
	res githubrel.Result
}

func (g *ghStub) Latest(context.Context, string) (githubrel.Result, error) { return g.res, nil }

type detStub struct {
	res detect.Result
}

func (d detStub) Detect(context.Context, string, string) detect.Result { return d.res }
