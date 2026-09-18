package githubrel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWatchesUpstreamNotFork(t *testing.T) {
	t.Parallel()
	want := "https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest"
	if LatestURL != want {
		t.Fatalf("LatestURL = %q, want %q", LatestURL, want)
	}
	if Owner != "DNSCrypt" || Repo != "dnscrypt-proxy" {
		t.Fatalf("owner/repo = %s/%s", Owner, Repo)
	}
	if strings.Contains(strings.ToLower(LatestURL), "78tacos") {
		t.Fatal("updater must watch upstream DNSCrypt/dnscrypt-proxy, not the 78tacos fork")
	}
}

func TestLatestParsesTagAndURL(t *testing.T) {
	t.Parallel()
	payload := map[string]any{
		"tag_name":     "2.1.18",
		"html_url":     "https://github.com/DNSCrypt/dnscrypt-proxy/releases/tag/2.1.18",
		"name":         "2.1.18",
		"body":         "notes",
		"draft":        false,
		"prerelease":   false,
		"published_at": "2026-07-18T12:14:35Z",
	}
	raw, _ := json.Marshal(payload)
	var gotETag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Error("unexpected If-None-Match on first fetch")
		}
		gotETag = `"abc123"`
		w.Header().Set("ETag", gotETag)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), URL: srv.URL, UserAgent: "dnscrypt-proxy-updater-test"}
	res, err := c.Latest(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Release.TagName != "2.1.18" {
		t.Fatalf("tag = %q", res.Release.TagName)
	}
	if !strings.Contains(res.Release.HTMLURL, "DNSCrypt/dnscrypt-proxy") {
		t.Fatalf("html_url = %q", res.Release.HTMLURL)
	}
	if res.ETag != gotETag {
		t.Fatalf("etag = %q", res.ETag)
	}
	if res.Release.PublishedAt.UTC() != time.Date(2026, 7, 18, 12, 14, 35, 0, time.UTC) {
		t.Fatalf("published_at = %v", res.Release.PublishedAt)
	}
}

func TestLatestUsesIfNoneMatch(t *testing.T) {
	t.Parallel()
	var saw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", saw)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	res, err := c.Latest(context.Background(), `W/"111"`)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotModified {
		t.Fatal("expected 304 NotModified")
	}
	if saw != `W/"111"` {
		t.Fatalf("If-None-Match = %q", saw)
	}
	if res.Release.TagName != "" {
		t.Fatalf("304 should not include a body tag, got %q", res.Release.TagName)
	}
}

func TestLatestHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"nope"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	_, err := c.Latest(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
}

func TestLatestMissingTag(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"html_url":"https://example"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), URL: srv.URL}
	if _, err := c.Latest(context.Background(), ""); err == nil {
		t.Fatal("expected error for missing tag_name")
	}
}
