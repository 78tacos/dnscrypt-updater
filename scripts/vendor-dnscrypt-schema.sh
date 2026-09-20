#!/usr/bin/env bash
# Vendor example config from an official DNSCrypt/dnscrypt-proxy tag.
# Usage: scripts/vendor-dnscrypt-schema.sh [tag]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TAG="${1:-}"
if [[ -z "$TAG" ]]; then
  TAG="$(curl -fsSL -H 'Accept: application/vnd.github+json' https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])')"
fi
DEST="$ROOT/internal/proxyconf/upstream"
mkdir -p "$DEST"
BASE="https://raw.githubusercontent.com/DNSCrypt/dnscrypt-proxy/${TAG}/dnscrypt-proxy"
for f in \
  example-dnscrypt-proxy.toml \
  example-allowed-ips.txt \
  example-allowed-names.txt \
  example-blocked-ips.txt \
  example-blocked-names.txt \
  example-captive-portals.txt \
  example-cloaking-rules.txt \
  example-forwarding-rules.txt
do
  curl -fsSL "$BASE/$f" -o "$DEST/$f"
done
curl -fsSL "$BASE/config.go" -o "$DEST/config.go.txt"
curl -fsSL "https://raw.githubusercontent.com/DNSCrypt/dnscrypt-proxy/${TAG}/LICENSE" -o "$DEST/LICENSE"
printf '%s\n' "$TAG" > "$DEST/VERSION"
echo "Vendored DNSCrypt/dnscrypt-proxy $TAG"
(cd "$ROOT" && go generate ./internal/proxyconf)
