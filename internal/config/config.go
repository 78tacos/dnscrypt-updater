package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	AppName         = "dnscrypt-proxy-updater"
	LegacyAppName   = "dnscrypt-updater"
	DefaultInterval = 12 * time.Hour
	MinInterval     = 15 * time.Minute
	SnoozeDuration  = 24 * time.Hour
)

// File is the user-editable settings document.
type File struct {
	// CheckInterval is a Go duration such as "12h" or "1h".
	CheckInterval string `json:"check_interval"`
	// BinaryPath, if set, is the dnscrypt-proxy executable to query.
	BinaryPath string `json:"binary_path"`
	// CurrentVersion is a manual override used instead of CLI detection.
	CurrentVersion string `json:"current_version"`
	// SkipVersion, if it matches the latest GitHub tag, suppresses notifications.
	SkipVersion string `json:"skip_version"`
	// SnoozeUntil is RFC3339; notifications are suppressed until this time.
	SnoozeUntil string `json:"snooze_until"`
	// Notify enables desktop notifications (tray status still updates).
	Notify bool `json:"notify"`
	// InstallDir, if set, is where -install places dnscrypt-proxy.
	InstallDir string `json:"install_dir"`
	// SetSystemDNS, on Windows, points connected adapters at 127.0.0.1 after install.
	SetSystemDNS bool `json:"set_system_dns"`
	// ManageService installs/starts the official dnscrypt-proxy Windows service.
	ManageService bool `json:"manage_service"`
}

// State is machine-written cache (ETag, last check).
type State struct {
	ETag                 string    `json:"etag"`
	CachedTag            string    `json:"cached_tag"`
	CachedHTMLURL        string    `json:"cached_html_url"`
	CachedPublishedAt    time.Time `json:"cached_published_at"`
	LastCheck            time.Time `json:"last_check"`
	LastNotifiedVersion  string    `json:"last_notified_version"`
	LastNotFoundNotified time.Time `json:"last_not_found_notified"`
	CachedAssetName      string    `json:"cached_asset_name"`
	CachedAssetURL       string    `json:"cached_asset_url"`
	CachedMinisigName    string    `json:"cached_minisig_name"`
	CachedMinisigURL     string    `json:"cached_minisig_url"`
}

// Paths locates config/state/log files.
type Paths struct {
	Dir     string
	File    string
	State   string
	Log     string
	Pending string
}

func DefaultFile() File {
	return File{
		CheckInterval: DefaultInterval.String(),
		Notify:        true,
		SetSystemDNS:  true,
		ManageService: true,
	}
}

func ResolvePaths(explicitConfig string) (Paths, error) {
	if strings.TrimSpace(explicitConfig) != "" {
		abs, err := filepath.Abs(explicitConfig)
		if err != nil {
			return Paths{}, err
		}
		dir := filepath.Dir(abs)
		return Paths{
			Dir:     dir,
			File:    abs,
			State:   filepath.Join(dir, "state.json"),
			Log:     filepath.Join(dir, AppName+".log"),
			Pending: filepath.Join(dir, "pending"),
		}, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, AppName)
	legacy := filepath.Join(base, LegacyAppName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if st, err := os.Stat(legacy); err == nil && st.IsDir() {
			dir = legacy
		}
	}
	return Paths{
		Dir:     dir,
		File:    filepath.Join(dir, "config.json"),
		State:   filepath.Join(dir, "state.json"),
		Log:     filepath.Join(dir, AppName+".log"),
		Pending: filepath.Join(dir, "pending"),
	}, nil
}

func (p Paths) EnsureDir() error {
	return os.MkdirAll(p.Dir, 0o755)
}

func LoadFile(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultFile(), nil
		}
		return File{}, err
	}
	cfg := DefaultFile()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return File{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func SaveFile(path string, cfg File) error {
	return writeJSON(path, cfg)
}

func LoadState(path string) (State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	return st, nil
}

func SaveState(path string, st State) error {
	return writeJSON(path, st)
}

func (f File) Interval() (time.Duration, error) {
	s := strings.TrimSpace(f.CheckInterval)
	if s == "" {
		return DefaultInterval, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("check_interval: %w", err)
	}
	if d < MinInterval {
		return MinInterval, nil
	}
	return d, nil
}

func (f File) Snoozed(now time.Time) bool {
	s := strings.TrimSpace(f.SnoozeUntil)
	if s == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return false
	}
	return now.Before(t)
}

func (f File) Skips(remoteTag string) bool {
	skip := strings.TrimSpace(f.SkipVersion)
	if skip == "" || remoteTag == "" {
		return false
	}
	// Compare normalized tags when both parse; otherwise exact/trim match.
	return strings.EqualFold(strings.TrimPrefix(strings.TrimPrefix(skip, "v"), "V"), strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(remoteTag), "v"), "V"))
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
