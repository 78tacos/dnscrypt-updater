package apply

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxAssetBytes = 80 << 20
	maxRedirects  = 10
)

func (a *Applier) httpClient() *http.Client {
	if a != nil && a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			if err := ValidateAssetURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

func (a *Applier) userAgent() string {
	if a != nil && a.UserAgent != "" {
		return a.UserAgent
	}
	return "dnscrypt-proxy-updater (+https://github.com/78tacos/dnscrypt-updater)"
}

// ValidateAssetURL allows only official GitHub / GitHub-release CDN hosts over HTTPS.
func ValidateAssetURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse download url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing non-https download url")
	}
	if !allowedDownloadHost(u.Host) {
		return fmt.Errorf("refusing download host %q", u.Host)
	}
	return nil
}

func allowedDownloadHost(host string) bool {
	h := strings.ToLower(host)
	switch h {
	case "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return true
	default:
		return strings.HasSuffix(h, ".githubusercontent.com")
	}
}

func (a *Applier) validateURL(raw string) error {
	if a != nil && a.AllowDownload != nil {
		return a.AllowDownload(raw)
	}
	return ValidateAssetURL(raw)
}

func (a *Applier) download(ctx context.Context, rawURL, dest string) error {
	if err := a.validateURL(rawURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", a.userAgent())
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", filepath.Base(dest), resp.StatusCode)
	}
	if resp.ContentLength > maxAssetBytes {
		return fmt.Errorf("download %s: Content-Length %d exceeds %d byte limit", filepath.Base(dest), resp.ContentLength, maxAssetBytes)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxAssetBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if n > maxAssetBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("download %s: body exceeds %d byte limit", filepath.Base(dest), maxAssetBytes)
	}
	if n == 0 {
		_ = os.Remove(tmp)
		return fmt.Errorf("download %s: empty body", filepath.Base(dest))
	}
	return os.Rename(tmp, dest)
}
