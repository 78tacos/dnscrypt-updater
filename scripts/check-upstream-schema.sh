#!/usr/bin/env bash
# Fail if official DNSCrypt/dnscrypt-proxy latest tag (or example toml hash)
# no longer matches the vendored schema pin.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PINNED="$(tr -d '[:space:]' < "$ROOT/internal/proxyconf/upstream/VERSION")"
EXPECTED="$(python3 -c 'import re,sys; text=open(sys.argv[1]).read(); m=re.search(r"GeneratedExampleSHA256\s*=\s*\"([0-9a-f]+)\"", text); print(m.group(1) if m else "")' "$ROOT/internal/proxyconf/catalog_gen.go")"
LATEST="$(curl -fsSL -H 'Accept: application/vnd.github+json' https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])')"
echo "vendored tag: $PINNED"
echo "github latest: $LATEST"
if [[ "$PINNED" != "$LATEST" ]]; then
  echo "Upstream dnscrypt-proxy $LATEST is newer than vendored $PINNED."
  echo "Refresh with: ./scripts/vendor-dnscrypt-schema.sh $LATEST && go generate ./internal/proxyconf"
  exit 1
fi
TMP="$(mktemp)"
curl -fsSL "https://raw.githubusercontent.com/DNSCrypt/dnscrypt-proxy/${LATEST}/dnscrypt-proxy/example-dnscrypt-proxy.toml" -o "$TMP"
GOT="$(sha256sum "$TMP" | awk '{print $1}')"
rm -f "$TMP"
echo "remote example sha256: $GOT"
echo "generated pin:         $EXPECTED"
if [[ -n "$EXPECTED" && "$GOT" != "$EXPECTED" ]]; then
  echo "example-dnscrypt-proxy.toml hash drifted at tag $LATEST"
  exit 1
fi
echo "schema pin is current"
