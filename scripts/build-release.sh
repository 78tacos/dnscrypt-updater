#!/usr/bin/env bash
# Build release archives for dnscrypt-updater. Usage: scripts/build-release.sh [version]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || true)"
fi
if [[ -z "$VERSION" ]]; then
  VERSION="dev"
fi
if [[ "$VERSION" != v* && "$VERSION" != dev* ]]; then
  VERSION="v${VERSION}"
fi
VER_NUM="${VERSION#v}"

LDFLAGS="-s -w -X github.com/78tacos/dnscrypt-updater/internal/app.AppVersion=${VER_NUM}"
WINDOWS_LDFLAGS="${LDFLAGS} -H=windowsgui"
OUT="${ROOT}/dist"
rm -rf "$OUT"
mkdir -p "$OUT"

copy_docs() {
  local tmp="$1"
  cp LICENSE README.md config.example.json "$tmp/"
}

build_windows() {
  local arch="$1"
  local tmp
  tmp="$(mktemp -d)"
  CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags "$WINDOWS_LDFLAGS" -o "${tmp}/dnscrypt-updater.exe" ./cmd/dnscrypt-updater
  copy_docs "$tmp"
  (cd "$tmp" && zip -q "${OUT}/dnscrypt-updater-${VERSION}-windows-${arch}.zip" dnscrypt-updater.exe LICENSE README.md config.example.json)
  rm -rf "$tmp"
}

build_unix() {
  local os="$1"
  local arch="$2"
  local tmp
  tmp="$(mktemp -d)"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "${tmp}/dnscrypt-updater" ./cmd/dnscrypt-updater
  copy_docs "$tmp"
  tar -C "$tmp" -czf "${OUT}/dnscrypt-updater-${VERSION}-${os}-${arch}.tar.gz" dnscrypt-updater LICENSE README.md config.example.json
  rm -rf "$tmp"
}

build_windows amd64
build_windows arm64
build_unix linux amd64
build_unix linux arm64
build_unix darwin amd64
build_unix darwin arm64

(cd "$OUT" && sha256sum -- * > SHA256SUMS)
ls -la "$OUT"
