# dnscrypt-updater

Lightweight **system-tray companion** that watches for new **official** [dnscrypt-proxy](https://github.com/DNSCrypt/dnscrypt-proxy) releases and **notifies** you.

v1 is **notify-only**. It never installs dnscrypt-proxy, never replaces a running service binary, never rewrites `dnscrypt-proxy.toml`, and never changes NIC DNS settings. It does **not** bundle or redistribute dnscrypt-proxy.

This is an original MIT-licensed companion. It is **not** SimpleDnsCrypt (a Windows management GUI). Study that project for UX ideas if you like; this repo does not copy it.

**Release source (hardcoded):** [`DNSCrypt/dnscrypt-proxy`](https://github.com/DNSCrypt/dnscrypt-proxy) via

`GET https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest`

Captain’s fork [`78tacos/dnscrypt-proxy`](https://github.com/78tacos/dnscrypt-proxy) is **not** watched. Prefer GitHub (or package managers that track it). [dnscrypt.org](https://www.dnscrypt.org/) warns against unofficial/torrent “DNSCrypt client” downloads.

---

## OS support

| OS | Tray app | Headless `-check-once` | Notes |
| --- | --- | --- | --- |
| **Windows** (primary) | Yes (no CGO) | Yes | Build with `-H=windowsgui` to hide the console. Notifications via Windows toasts. |
| Linux | Nice-to-have | Yes (default build) | Tray needs CGO plus GTK3 / AppIndicator headers and `-tags systray`. Untagged Linux builds fall back to a one-shot check. |
| macOS | Nice-to-have | Yes | Tray needs CGO and a proper `.app` bundle for a reliable menu-bar extra. Untagged builds fall back to a one-shot check. |

Version detection tries `dnscrypt-proxy -version` then `--version` (wiki / common CLI). It does **not** trust Windows PE `FileVersion`, which is often empty ([dnscrypt-proxy#2381](https://github.com/DNSCrypt/dnscrypt-proxy/issues/2381)).

---

## Safety (v1)

- Poll GitHub Releases and compare to the local CLI version (or your override).
- If remote > local: desktop notification + tray badge/menu. The action **Open GitHub release page** opens the official tag page.
- No auto-install, no zip apply, no minisign-and-swap, no service restart.
- If the binary is missing, the app reports **not found** and does not invent a version. Set `binary_path` or `current_version`.
- Official assets are minisign-signed (`RWTk1xXqcTODeYttYMCMLo0YJHaFEHn7a3akqHlb/7QvIQXHVPxKbjB5`). Verification belongs in a future *apply* flow, not v1.

---

## How to run

### From source (any OS, one-shot check)

Requires [Go 1.22+](https://go.dev/dl/).

```bash
git clone https://github.com/78tacos/dnscrypt-updater.git
cd dnscrypt-updater
go test ./...
go build -o dnscrypt-updater ./cmd/dnscrypt-updater
./dnscrypt-updater -check-once
./dnscrypt-updater -check-once -json
./dnscrypt-updater -check-once -current-version 2.1.14
```

`-check-once` exit codes: `0` up to date / skipped / snoozed, `1` error, `2` update available, `3` dnscrypt-proxy not found.

Config is created on first run under the OS user config dir:

| OS | Default config |
| --- | --- |
| Windows | `%APPDATA%\dnscrypt-updater\config.json` |
| Linux | `~/.config/dnscrypt-updater/config.json` |
| macOS | `~/Library/Application Support/dnscrypt-updater/config.json` |

Logs: `dnscrypt-updater.log` in that directory. GitHub `ETag` / `If-None-Match` cache lives in `state.json`.

### Windows tray binary

Build **on Windows** (or cross-compile from Linux/macOS; no CGO):

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H=windowsgui -s -w" -o dnscrypt-updater.exe ./cmd/dnscrypt-updater
```

ARM64:

```bash
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-H=windowsgui -s -w" -o dnscrypt-updater-arm64.exe ./cmd/dnscrypt-updater
```

Put `dnscrypt-updater.exe` anywhere and run it. A tray icon appears; the menu shows local vs GitHub, **Check now**, **Open GitHub release page**, **Skip this version**, **Snooze 24 hours**, **Quit**.

### Linux / macOS tray (optional)

Linux packages (Debian/Ubuntu): `libgtk-3-dev` and `libayatana-appindicator3-dev` (or `libappindicator3-dev`). Then:

```bash
CGO_ENABLED=1 go build -tags systray -o dnscrypt-updater ./cmd/dnscrypt-updater
```

macOS: `CGO_ENABLED=1 go build -tags systray` and wrap the binary in a minimal `.app` bundle so the extra stays in the menu bar.

---

## Configuration

See [`config.example.json`](config.example.json).

| Field | Default | Meaning |
| --- | --- | --- |
| `check_interval` | `12h` | How often to poll GitHub. Go duration (`1h`, `12h`, …). Minimum `15m` so we do not hammer the API. |
| `binary_path` | empty | Exact `dnscrypt-proxy` / `dnscrypt-proxy.exe` to run `-version` on. |
| `current_version` | empty | **Manual override** used for comparison when set (still tries to show the discovered path). |
| `skip_version` | empty | Do not notify for this GitHub tag. |
| `snooze_until` | empty | RFC3339 timestamp; suppress notifications until then. |
| `notify` | `true` | Desktop notifications. Tray status still updates. |

CLI overrides for one run: `-current-version`, `-binary-path`, `-config`, `-no-notify`, `-notify` (with `-check-once`), `-quiet`.

### Quiet / start with the OS

v1 does not register itself as a login item. Notes:

**Windows**

- Build with `-H=windowsgui` so there is no console (“quiet”).
- Start with Windows: copy a shortcut to `shell:startup`, or a Task Scheduler task (“At log on”, hidden, action = `dnscrypt-updater.exe`).
- Do not run it as a Windows service unless you know you need that; this is a per-user tray app.

**Linux**

- XDG Autostart: `~/.config/autostart/dnscrypt-updater.desktop` with `Exec=/path/to/dnscrypt-updater`.
- systemd user unit `Type=simple` / `WantedBy=default.target` also works for `-check-once` loops, but the tray binary should run inside a graphical session.

**macOS**

- Login Item, or a LaunchAgent with `RunAtLoad`. Menu-bar extras belong in an `.app`.

---

## How version detection works

1. If `current_version` / `-current-version` is set, that value is compared to GitHub `tag_name` (optional `v` prefix is ignored).
2. Otherwise locate `dnscrypt-proxy`:
   - configured `binary_path`
   - `PATH`
   - common dirs (`/opt/dnscrypt-proxy/` on Linux as in the wiki updater, Program Files / Scoop / Chocolatey on Windows, Homebrew paths on macOS)
   - Windows `sc qc dnscrypt-proxy` `BINARY_PATH_NAME`, or Linux `systemctl show -p ExecStart`
3. Run `dnscrypt-proxy -version`, then `--version`. Parse stdout. **PE FileVersion is not used.**
4. Distro packages may lag GitHub. The tray shows “local binary vs GitHub”; it does not claim your package manager is current.

---

## Development

```bash
go test ./...
```

Version compare lives in `internal/version` and is covered by table-driven tests (prefix `v`, pre-release ordering, build metadata). GitHub ETag handling, CLI parsing, skip/snooze, and “do not invent a version” are unit-tested with fakes — no live network required.

Live poll (hits GitHub, 60 unauthenticated requests/hour; ETag caching keeps this cheap):

```bash
go run ./cmd/dnscrypt-updater -check-once -current-version 2.1.0
```

---

## License

[MIT](LICENSE)
