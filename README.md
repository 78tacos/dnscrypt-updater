# dnscrypt-proxy-updater

Tray companion that watches official [dnscrypt-proxy](https://github.com/DNSCrypt/dnscrypt-proxy) releases, **downloads the minisign-signed archive**, and can **install it for the system**.

This is an original MIT-licensed companion. It is **not** SimpleDnsCrypt. It only installs binaries published by [`DNSCrypt/dnscrypt-proxy`](https://github.com/DNSCrypt/dnscrypt-proxy). Captain’s fork [`78tacos/dnscrypt-proxy`](https://github.com/78tacos/dnscrypt-proxy) is **not** watched.

**Release source (hardcoded):**

`GET https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest`

[dnscrypt.org](https://www.dnscrypt.org/) warns against unofficial/torrent “DNSCrypt client” downloads. Prefer GitHub (or package managers that track it).

---

## What install does

Triggered from the tray (**Install / Update dnscrypt-proxy**) or:

```bash
dnscrypt-proxy-updater -install
```

On Windows this:

1. Downloads the official `dnscrypt-proxy-win64-*.zip` (or `winarm`) **and** the matching `.minisig`.
2. Verifies the archive with minisign and pubkey `RWTk1xXqcTODeYttYMCMLo0YJHaFEHn7a3akqHlb/7QvIQXHVPxKbjB5` (also DNSSEC TXT `dnscrypt-proxy.key.dnscrypt.info`).
3. Installs to `%ProgramFiles%\dnscrypt-proxy` (or your existing install dir). The live `dnscrypt-proxy.toml` is never overwritten; a first install copies `example-dnscrypt-proxy.toml`. The previous exe is kept as `dnscrypt-proxy.exe.old`.
4. Registers and starts the official `dnscrypt-proxy -service` (needs Administrator; the tray prompts UAC).
5. Points connected adapters with a default gateway at `127.0.0.1` so the OS uses the local proxy (wiki step 3). Pass `-no-dns` to skip.

It does **not** rewrite your toml on install, does **not** follow forks, and does **not** auto-install on a background poll. You have to click Install or pass `-install`. The settings UI (below) can patch toml after install, while leaving unknown keys and comments in place.

Linux/macOS: files go to `/opt/dnscrypt-proxy` (needs root). Automatic DNS change is Windows-only.

---

## Settings UI

Tray **Configure dnscrypt-proxy…** (or `-configure`) opens a **127.0.0.1** page in the default browser. It is not SimpleDnsCrypt and it is not the proxy’s own `[monitoring_ui]` dashboard.

- Forms are generated from the vendored official `example-dnscrypt-proxy.toml` (pinned DNSCrypt/dnscrypt-proxy tag in [`internal/proxyconf/upstream/VERSION`](internal/proxyconf/upstream/VERSION)).
- **Easy** tab: common keys, presets (overlays, not a full rewrite), clickable examples from upstream comments, and dismissible suggestions.
- **Advanced** tab: the full generated catalog.
- **List files**: `forwarding-rules.txt`, `cloaking-rules.txt`, block/allow lists, captive-portal map. Auto-downloaded `public-resolvers.md` / relays caches are not rewritten.
- Save writes a backup (`*.bak`), patches keys in place, runs `dnscrypt-proxy -check`, then restarts the service. On Windows, Program Files writes re-use the existing UAC prompt (`-apply-config`).

```bash
dnscrypt-proxy-updater -configure
```

When official dnscrypt-proxy adds or changes config keys, refresh the pin and rebuild this updater:

```bash
./scripts/vendor-dnscrypt-schema.sh          # latest official tag
# or: ./scripts/vendor-dnscrypt-schema.sh 2.1.18
go generate ./internal/proxyconf
go test ./...
```

CI job `schema-drift` fails when GitHub’s latest official tag no longer matches the vendored example toml.

---

## OS support

| OS | Tray app | Headless `-check-once` / `-install` | Notes |
| --- | --- | --- | --- |
| **Windows** (primary) | Yes (no CGO) | Yes | Build with `-H=windowsgui` to hide the console. Notifications via Windows toasts. |
| Linux | Nice-to-have | Yes (default build) | Tray needs CGO plus GTK3 / AppIndicator headers and `-tags systray`. Untagged Linux builds fall back to a one-shot check. |
| macOS | Nice-to-have | Yes | Tray needs CGO and a proper `.app` bundle for a reliable menu-bar extra. Untagged builds fall back to a one-shot check. |

Version detection tries `dnscrypt-proxy -version` then `--version`. It does **not** trust Windows PE `FileVersion`, which is often empty ([dnscrypt-proxy#2381](https://github.com/DNSCrypt/dnscrypt-proxy/issues/2381)).

---

## How to run

### Prebuilt binaries

GitHub Releases publish `dnscrypt-proxy-updater.exe` zips for Windows amd64/arm64 (tray, no console) and headless archives for Linux/macOS.

1. Open the latest [GitHub Release](https://github.com/78tacos/dnscrypt-updater/releases/latest).
2. Windows: unzip `dnscrypt-proxy-updater-*-windows-amd64.zip` (or `windows-arm64` on ARM PCs) and run `dnscrypt-proxy-updater.exe`.
3. Use tray **Install / Update dnscrypt-proxy** (or `-install`) to download the official signed proxy and install it for the system.
4. Linux/macOS: extract the `.tar.gz` and run `./dnscrypt-proxy-updater -check-once` (tray builds still need CGO; see below).

### From source (any OS)

Requires [Go 1.22+](https://go.dev/dl/).

```bash
git clone https://github.com/78tacos/dnscrypt-updater.git
cd dnscrypt-updater
go test ./...
go build -o dnscrypt-proxy-updater ./cmd/dnscrypt-proxy-updater
./dnscrypt-proxy-updater -check-once
./dnscrypt-proxy-updater -install
```

`-check-once` exit codes: `0` up to date / skipped / snoozed, `1` error, `2` update available, `3` dnscrypt-proxy not found.

Config is created on first run under the OS user config dir (`dnscrypt-proxy-updater`). Existing `dnscrypt-updater` config directories are still used if present.

| OS | Default config |
| --- | --- |
| Windows | `%APPDATA%\dnscrypt-proxy-updater\config.json` |
| Linux | `~/.config/dnscrypt-proxy-updater/config.json` |
| macOS | `~/Library/Application Support/dnscrypt-proxy-updater/config.json` |

Logs: `dnscrypt-proxy-updater.log` in that directory. GitHub `ETag` / `If-None-Match` cache lives in `state.json`.

### Windows tray binary

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-H=windowsgui -s -w" -o dnscrypt-proxy-updater.exe ./cmd/dnscrypt-proxy-updater
```

ARM64:

```bash
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-H=windowsgui -s -w" -o dnscrypt-proxy-updater-arm64.exe ./cmd/dnscrypt-proxy-updater
```

Run `dnscrypt-proxy-updater.exe`. The tray menu shows local vs GitHub, the official signed archive, **Check now**, **Install / Update dnscrypt-proxy**, **Configure dnscrypt-proxy…**, **Open GitHub release page**, **Skip this version**, **Snooze 24 hours**, **Quit**.

Windows archives on GitHub are `win64` / `win32` / `winarm` zips (plus unsigned `.msi` files, which this app ignores because they have no `.minisig`).

### Linux / macOS tray (optional)

Linux packages (Debian/Ubuntu): `libgtk-3-dev` and `libayatana-appindicator3-dev` (or `libappindicator3-dev`). Then:

```bash
CGO_ENABLED=1 go build -tags systray -o dnscrypt-proxy-updater ./cmd/dnscrypt-proxy-updater
```

---

## Configuration

See [`config.example.json`](config.example.json).

| Field | Default | Meaning |
| --- | --- | --- |
| `check_interval` | `12h` | How often to poll GitHub. Go duration (`1h`, `12h`, …). Minimum `15m`. |
| `binary_path` | empty | Exact `dnscrypt-proxy` / `dnscrypt-proxy.exe` to run `-version` on. Set automatically after `-install`. |
| `current_version` | empty | Manual override used for comparison when set. |
| `skip_version` | empty | Do not notify for this GitHub tag. |
| `snooze_until` | empty | RFC3339 timestamp; suppress notifications until then. |
| `notify` | `true` | Desktop notifications. Tray status still updates. |
| `install_dir` | empty | Override install location. Windows default: `%ProgramFiles%\dnscrypt-proxy`. |
| `set_system_dns` | `true` | After install on Windows, set connected adapters to `127.0.0.1`. |
| `manage_service` | `true` | Install/start `dnscrypt-proxy -service`. |

CLI: `-install`, `-configure`, `-apply-config` (internal, after UAC), `-no-dns`, `-no-service`, `-install-dir`, `-current-version`, `-binary-path`, `-config`, `-no-notify`, `-notify` (with `-check-once`), `-quiet`.

### Quiet / start with the OS

**Windows**

- Release builds use `-H=windowsgui` so there is no console.
- Start with Windows: copy a shortcut to `shell:startup`, or a Task Scheduler task (“At log on”, hidden, action = `dnscrypt-proxy-updater.exe`).
- Installing the proxy as a service still requires Administrator (UAC).

**Linux**

- XDG Autostart: `~/.config/autostart/dnscrypt-proxy-updater.desktop`.

**macOS**

- Login Item, or a LaunchAgent with `RunAtLoad`.

---

## How version detection works

1. If `current_version` / `-current-version` is set, that value is compared to GitHub `tag_name` (optional `v` prefix is ignored).
2. Otherwise locate `dnscrypt-proxy`:
   - configured `binary_path`
   - `PATH`
   - common dirs (`/opt/dnscrypt-proxy/` on Linux as in the wiki updater, Program Files / Scoop / Chocolatey on Windows, Homebrew paths on macOS)
   - Windows `sc qc dnscrypt-proxy` `BINARY_PATH_NAME`, or Linux `systemctl show -p ExecStart`
3. Run `dnscrypt-proxy -version`, then `--version`. Parse stdout. **PE FileVersion is not used.**
4. Distro packages may lag GitHub. The tray shows the **local binary CLI version** vs **GitHub `tag_name`**. If the current binary is a Scoop/Chocolatey/apt path, `-install` writes a separate copy under Program Files / `/opt` instead of replacing the package.

---

## Official signed assets

| This OS | Asset name pattern |
| --- | --- |
| Windows amd64 | `dnscrypt-proxy-win64-<tag>.zip` + `.minisig` |
| Windows 386 | `dnscrypt-proxy-win32-<tag>.zip` + `.minisig` |
| Windows arm64 | `dnscrypt-proxy-winarm-<tag>.zip` + `.minisig` |
| Linux amd64 | `dnscrypt-proxy-linux_x86_64-<tag>.tar.gz` + `.minisig` |
| macOS arm64 / Intel | `dnscrypt-proxy-macos_arm64-…` / `macos_x86_64-…` |

Downloads are only accepted from `github.com` and `*.githubusercontent.com`. MSI installers are ignored (unsigned on GitHub Releases).

---

## Development

```bash
go test ./...
go generate ./internal/proxyconf
```

Version compare lives in `internal/version` and is covered by table-driven tests. GitHub ETag handling, CLI parsing, skip/snooze, minisign verify, zip extract, and install file copy are unit-tested with fakes — no live network required.

Live poll (hits GitHub, 60 unauthenticated requests/hour; ETag caching keeps this cheap):

```bash
go run ./cmd/dnscrypt-proxy-updater -check-once -current-version 2.1.0
```

---

## License

[MIT](LICENSE)
