package check

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/detect"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
	"github.com/78tacos/dnscrypt-updater/internal/version"
)

// Result is one poll of local vs official GitHub latest.
type Result struct {
	LocalVersion     string `json:"local_version"`
	LocalSource      string `json:"local_source"`
	BinaryPath       string `json:"binary_path,omitempty"`
	RemoteVersion    string `json:"remote_version"`
	ReleaseURL       string `json:"release_url"`
	PublishedAt      string `json:"published_at,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Skipped          bool   `json:"skipped"`
	Snoozed          bool   `json:"snoozed"`
	NotFound         bool   `json:"not_found"`
	NotModified      bool   `json:"not_modified"`
	ShouldNotify     bool   `json:"should_notify"`
	NotifyReason     string `json:"notify_reason,omitempty"`
	Message          string `json:"message"`
	LocalParseError  string `json:"local_parse_error,omitempty"`
	RemoteFetchError string `json:"remote_fetch_error,omitempty"`
	OfficialAsset    string `json:"official_asset,omitempty"`
	OfficialAssetURL string `json:"official_asset_url,omitempty"`
	MinisigName      string `json:"minisig_name,omitempty"`
	MinisigURL       string `json:"minisig_url,omitempty"`
	MinisignPubKey   string `json:"minisign_pubkey,omitempty"`
}

// GitHubClient is satisfied by githubrel.Client.
type GitHubClient interface {
	Latest(ctx context.Context, etag string) (githubrel.Result, error)
}

// Detector is satisfied by detect.Runner.
type Detector interface {
	Detect(ctx context.Context, configuredPath, override string) detect.Result
}

// Engine runs a single check and updates state.
type Engine struct {
	GitHub GitHubClient
	Detect Detector
	Now    func() time.Time
	GOOS   string
	GOARCH string
}

func (e Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Run detects the local binary, fetches upstream latest, and decides notify.
// forceNotify is true for an explicit "Check now" (re-notify even if already shown).
func (e Engine) Run(ctx context.Context, cfg config.File, st config.State, forceNotify bool) (Result, config.State, error) {
	now := e.now()
	out := Result{}
	st.LastCheck = now

	loc := e.Detect.Detect(ctx, cfg.BinaryPath, cfg.CurrentVersion)
	out.BinaryPath = loc.Path
	out.LocalSource = loc.Source
	if loc.Found {
		out.LocalVersion = loc.Version.String()
	} else {
		out.NotFound = true
		if loc.Err != nil {
			out.LocalParseError = loc.Err.Error()
		}
	}

	gh, err := e.GitHub.Latest(ctx, st.ETag)
	if err != nil {
		out.RemoteFetchError = err.Error()
		out.Message = "Failed to check GitHub releases: " + err.Error()
		if st.CachedTag != "" {
			out.RemoteVersion = version.Normalize(st.CachedTag)
			out.ReleaseURL = st.CachedHTMLURL
			out.OfficialAsset = st.CachedAssetName
			out.OfficialAssetURL = st.CachedAssetURL
			out.MinisigName = st.CachedMinisigName
			out.MinisigURL = st.CachedMinisigURL
			out.MinisignPubKey = githubrel.MinisignPubKey
		}
		return out, st, err
	}

	rel := gh.Release
	if gh.NotModified {
		out.NotModified = true
		rel.TagName = st.CachedTag
		rel.HTMLURL = st.CachedHTMLURL
		rel.PublishedAt = st.CachedPublishedAt
	} else {
		st.ETag = gh.ETag
		st.CachedTag = rel.TagName
		st.CachedHTMLURL = rel.HTMLURL
		st.CachedPublishedAt = rel.PublishedAt
	}

	out.RemoteVersion = version.Normalize(rel.TagName)
	out.ReleaseURL = rel.HTMLURL
	if !rel.PublishedAt.IsZero() {
		out.PublishedAt = rel.PublishedAt.UTC().Format(time.RFC3339)
	}
	e.fillOfficialAsset(&out, &st, rel)

	if out.NotFound {
		out.Message = "dnscrypt-proxy not found. Set binary_path or current_version in config. No version was invented."
		out.ShouldNotify, out.NotifyReason = shouldNotifyNotFound(cfg, st, now, forceNotify)
		return out, st, nil
	}

	cmp, err := version.CompareStrings(out.RemoteVersion, out.LocalVersion)
	if err != nil {
		out.Message = "Could not compare versions: " + err.Error()
		return out, st, err
	}
	out.UpdateAvailable = cmp > 0
	out.Skipped = out.UpdateAvailable && cfg.Skips(rel.TagName)
	out.Snoozed = out.UpdateAvailable && cfg.Snoozed(now)

	switch {
	case !out.UpdateAvailable:
		out.Message = fmt.Sprintf("Up to date (local %s, GitHub %s).", out.LocalVersion, out.RemoteVersion)
	case out.Skipped:
		out.Message = fmt.Sprintf("Update %s available but skipped.", out.RemoteVersion)
	case out.Snoozed:
		out.Message = fmt.Sprintf("Update %s available (snoozed).", out.RemoteVersion)
	default:
		out.Message = fmt.Sprintf("Update available: local %s → GitHub %s.", out.LocalVersion, out.RemoteVersion)
	}

	out.ShouldNotify, out.NotifyReason = decideNotify(out, cfg, st, forceNotify)
	return out, st, nil
}

func (e Engine) fillOfficialAsset(out *Result, st *config.State, rel githubrel.Release) {
	out.MinisignPubKey = githubrel.MinisignPubKey
	goos, goarch := e.GOOS, e.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if signed, ok := githubrel.SelectSignedArchive(goos, goarch, rel.Assets); ok {
		out.OfficialAsset = signed.Archive.Name
		out.OfficialAssetURL = signed.Archive.BrowserDownloadURL
		out.MinisigName = signed.Minisig.Name
		out.MinisigURL = signed.Minisig.BrowserDownloadURL
		st.CachedAssetName = out.OfficialAsset
		st.CachedAssetURL = out.OfficialAssetURL
		st.CachedMinisigName = out.MinisigName
		st.CachedMinisigURL = out.MinisigURL
		return
	}
	out.OfficialAsset = st.CachedAssetName
	out.OfficialAssetURL = st.CachedAssetURL
	out.MinisigName = st.CachedMinisigName
	out.MinisigURL = st.CachedMinisigURL
}

func decideNotify(res Result, cfg config.File, st config.State, force bool) (bool, string) {
	if !cfg.Notify {
		return false, "notify_disabled"
	}
	if !res.UpdateAvailable {
		return false, "up_to_date"
	}
	if res.Skipped {
		return false, "skipped"
	}
	if res.Snoozed && !force {
		return false, "snoozed"
	}
	if !force && version.Normalize(st.LastNotifiedVersion) == res.RemoteVersion {
		return false, "already_notified"
	}
	return true, "update_available"
}

func shouldNotifyNotFound(cfg config.File, st config.State, now time.Time, force bool) (bool, string) {
	if !cfg.Notify {
		return false, "notify_disabled"
	}
	if force {
		return true, "not_found"
	}
	if !st.LastNotFoundNotified.IsZero() && now.Sub(st.LastNotFoundNotified) < 24*time.Hour {
		return false, "not_found_recently_notified"
	}
	return true, "not_found"
}

// ErrNoGitHub is reserved for wiring mistakes.
var ErrNoGitHub = errors.New("github client not configured")
