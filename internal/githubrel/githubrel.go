// Package githubrel fetches the latest official dnscrypt-proxy GitHub release.
//
// The watched repository is hardcoded to DNSCrypt/dnscrypt-proxy (upstream).
// This companion must not follow forks such as 78tacos/dnscrypt-proxy.
package githubrel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// Owner and Repo identify the official upstream project.
	Owner = "DNSCrypt"
	Repo  = "dnscrypt-proxy"

	// LatestURL is the GitHub Releases API for the latest upstream tag.
	LatestURL = "https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest"

	// ReleasePage is the human-facing releases list (used if html_url is empty).
	ReleasePage = "https://github.com/DNSCrypt/dnscrypt-proxy/releases"

	apiVersion = "2022-11-28"
	maxBody    = 1 << 20
)

// Release is the subset of the GitHub release payload we need.
type Release struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Result is a fetch of /releases/latest, including ETag caching.
type Result struct {
	Release     Release
	ETag        string
	NotModified bool
	StatusCode  int
}

// Client talks to the GitHub Releases API.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	URL       string // tests may override; production leaves this empty
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) endpoint() string {
	if c != nil && c.URL != "" {
		return c.URL
	}
	return LatestURL
}

func (c *Client) userAgent() string {
	if c != nil && c.UserAgent != "" {
		return c.UserAgent
	}
	return "dnscrypt-updater/0.1.0 (+https://github.com/78tacos/dnscrypt-updater)"
}

// Latest GETs /releases/latest. If etag is non-empty it is sent as If-None-Match.
// A 304 response returns NotModified=true and an empty Release.
func (c *Client) Latest(ctx context.Context, etag string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", c.userAgent())
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.http().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	out := Result{
		ETag:       resp.Header.Get("ETag"),
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusNotModified {
		out.NotModified = true
		if out.ETag == "" {
			out.ETag = etag
		}
		return out, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return out, err
	}
	if len(body) > maxBody {
		return out, fmt.Errorf("github release response too large")
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return out, fmt.Errorf("github releases/latest: HTTP %d %s", resp.StatusCode, msg)
	}

	if err := json.Unmarshal(body, &out.Release); err != nil {
		return out, fmt.Errorf("decode github release: %w", err)
	}
	if out.Release.TagName == "" {
		return out, fmt.Errorf("github release missing tag_name")
	}
	if out.Release.HTMLURL == "" {
		out.Release.HTMLURL = ReleasePage + "/tag/" + out.Release.TagName
	}
	return out, nil
}
