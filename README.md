# dnscrypt-proxy-updater

Tray app for [dnscrypt-proxy](https://github.com/DNSCrypt/dnscrypt-proxy). It watches upstream releases, downloads the minisign-signed archive, and can install it.

This is not SimpleDnsCrypt. Installs come only from [`DNSCrypt/dnscrypt-proxy`](https://github.com/DNSCrypt/dnscrypt-proxy), using `https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest`. The fork at [`78tacos/dnscrypt-proxy`](https://github.com/78tacos/dnscrypt-proxy) is not followed.

[dnscrypt.org](https://www.dnscrypt.org/) warns against unofficial client downloads. Use the GitHub releases, or a package manager that tracks them.

---

## What install does

Triggered from the tray (**Install / Update dnscrypt-proxy**) or:

```bash
dnscrypt-proxy-updater -install
```

On Windows this:

1. Downloads `dnscrypt-proxy-win64-*.zip` (or `winarm`) and the matching `.minisig`.
2. Checks the archive with minisign. Public key: `RWTk1xXqcTODeYttYMCMLo0YJHaFEHn7a3akqHlb/7QvIQXHVPxKbjB5` (also the DNSSEC TXT `dnscrypt-proxy.key.dnscrypt.info`).
3. Installs under `%ProgramFiles%\dnscrypt-proxy`, or the existing install directory. An existing `dnscrypt-proxy.toml` is left alone; a first install copies `example-dnscrypt-proxy.toml`. The previous exe is kept as `dnscrypt-proxy.exe.old`.
4. Registers and starts the official `dnscrypt-proxy -service` (needs Administrator; the tray prompts UAC).
5. Points connected adapters with a default gateway at `127.0.0.1`, and at `::1` when IPv6 is bound (`ms_tcpip6`). Adapters without IPv6 stay on IPv4. This step is on by default. Pass `-no-dns` or `-no-system-dns`, or set `set_system_dns` to `false`, to skip.

It does **not** rewrite your toml, does **not** follow forks, and does **not** auto-install on a background poll. You have to click Install or pass `-install`.

Linux/macOS: files go to `/opt/dnscrypt-proxy` (needs root). Automatic DNS change is Windows-only.

---

## OS support

| OS | Tray app | Headless `-check-once` / `-install` | Notes |
| --- | --- | --- | --- |
| Windows (primary) | Yes (no CGO) | Yes | Build with `-H=windowsgui` to hide the console. Notifications use Windows toasts. |
| Linux | Optional | Yes (default build) | Tray needs CGO, GTK3 / AppIndicator headers, and `-tags systray`. Without those, the binary runs one check and exits. |
| macOS | Optional | Yes | Tray needs CGO and a `.app` bundle for a reliable menu-bar extra. Without that, the binary runs one check and exits. |

Version detection runs `dnscrypt-proxy -version`, then `--version`. Windows PE `FileVersion` is ignored; it is often empty ([dnscrypt-proxy#2381](https://github.com/DNSCrypt/dnscrypt-proxy/issues/2381)).

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

Run `dnscrypt-proxy-updater.exe`. The tray menu shows local vs GitHub, the official signed archive, **Check now**, **Install / Update dnscrypt-proxy**, **Open GitHub release page**, **Skip this version**, **Snooze 24 hours**, **Quit**.

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
| `set_system_dns` | `true` | After install on Windows, set connected adapters to `127.0.0.1` and to `::1` when IPv6 is enabled. Omit the field to keep the default. `false` opts out. |
| `manage_service` | `true` | Install/start `dnscrypt-proxy -service`. |

CLI: `-install`, `-no-dns`, `-no-system-dns` (same as `-no-dns`), `-no-service`, `-install-dir`, `-current-version`, `-binary-path`, `-config`, `-no-notify`, `-notify` (with `-check-once`), `-quiet`.

`set_system_dns` defaults to on, including when the field is left out of `config.json` or is JSON `null`. [`config.example.json`](config.example.json) shows `"set_system_dns": true`. An explicit `false` is kept. `-no-dns` and `-no-system-dns` skip the change for that run even when the config value is true.

### Quiet / start with the OS

**Windows**

- Release builds use `-H=windowsgui` so there is no console.
- Start with Windows: copy a shortcut to `shell:startup`, or a Task Scheduler task ("At log on", hidden, action = `dnscrypt-proxy-updater.exe`).
- Installing the proxy as a service still requires Administrator (UAC).

**Linux**

- XDG Autostart: `~/.config/autostart/dnscrypt-proxy-updater.desktop`.

**macOS**

- Login Item, or a LaunchAgent with `RunAtLoad`.

---

## How version detection works

If `current_version` or `-current-version` is set, that string is compared with the GitHub `tag_name` (a leading `v` is ignored). Otherwise the app looks up `dnscrypt-proxy` from `binary_path`, `PATH`, and the usual install locations: `/opt/dnscrypt-proxy/` (the path in the upstream wiki updater), Program Files, Scoop, and Chocolatey on Windows, and Homebrew on macOS. On Windows it also reads `BINARY_PATH_NAME` from `sc qc dnscrypt-proxy`; on Linux it reads `systemctl show -p ExecStart`.

The version comes from `dnscrypt-proxy -version`, then `--version`. Windows PE `FileVersion` is ignored. The tray shows that CLI version next to the GitHub tag. If the binary sits on a package-manager path (Scoop, Chocolatey, apt, and similar), `-install` leaves it alone and writes a separate copy under Program Files or `/opt`.

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
```

Version compare lives in `internal/version` and is covered by table-driven tests. GitHub ETag handling, CLI parsing, skip/snooze, minisign verify, zip extract, and install file copy are unit-tested with fakes — no live network required.

Live poll (hits GitHub, 60 unauthenticated requests/hour; ETag caching keeps this cheap):

```bash
go run ./cmd/dnscrypt-proxy-updater -check-once -current-version 2.1.0
```

---

## License

[MIT](LICENSE)
