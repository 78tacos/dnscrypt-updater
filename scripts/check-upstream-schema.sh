#!/usr/bin/env bash
# Fail if official DNSCrypt/dnscrypt-proxy latest tag (or example toml hash)
# no longer matches the vendored schema pin.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GITHUB_RELEASES_URL="https://api.github.com/repos/DNSCrypt/dnscrypt-proxy/releases/latest"
# Initial try plus a couple of retries for rate limits and server errors.
GITHUB_API_ATTEMPTS="${GITHUB_API_ATTEMPTS:-3}"
GITHUB_API_RETRY_DELAY="${GITHUB_API_RETRY_DELAY:-2}"

github_api_error_text() {
  python3 - "$1" <<'PY'
import json, sys
raw = open(sys.argv[1], encoding="utf-8", errors="replace").read().strip()
if not raw:
    print("empty response body")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except json.JSONDecodeError:
    print(" ".join(raw.split())[:300])
    raise SystemExit(0)
if isinstance(data, dict) and data.get("message"):
    print(str(data["message"]))
else:
    print(" ".join(raw.split())[:300])
PY
}

# Prints the latest release tag on stdout. Retries HTTP 403 and 5xx.
# Error bodies are reported as text. A non-JSON body cannot raise an uncaught JSONDecodeError.
fetch_latest_release_tag() {
  local -a curl_args=(
    -sS
    -H "Accept: application/vnd.github+json"
    -H "X-GitHub-Api-Version: 2022-11-28"
  )
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl_args+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
  fi

  local attempt=1 body code curl_ec msg parse_ec
  while [[ "$attempt" -le "$GITHUB_API_ATTEMPTS" ]]; do
    body="$(mktemp)"
    set +e
    code="$(curl "${curl_args[@]}" -o "$body" -w "%{http_code}" "$GITHUB_RELEASES_URL")"
    curl_ec=$?
    set -e
    if [[ "$curl_ec" -ne 0 ]]; then
      rm -f "$body"
      echo "GitHub API request failed (attempt ${attempt}/${GITHUB_API_ATTEMPTS}): curl exit ${curl_ec}." >&2
      if [[ "$attempt" -eq "$GITHUB_API_ATTEMPTS" ]]; then
        echo "GitHub API request failed after ${GITHUB_API_ATTEMPTS} attempts (curl exit ${curl_ec})." >&2
        return 1
      fi
      sleep "$GITHUB_API_RETRY_DELAY"
      attempt=$((attempt + 1))
      continue
    fi
    code="${code//$'\r'/}"
    code="${code//$'\n'/}"
    if [[ "$code" == "200" ]]; then
      set +e
      python3 - "$body" <<'PY'
import json, sys
raw = open(sys.argv[1], encoding="utf-8", errors="replace").read()
try:
    data = json.loads(raw)
except json.JSONDecodeError as exc:
    sys.stderr.write(
        f"GitHub API returned HTTP 200 but the body was not JSON ({exc}).\n"
    )
    raise SystemExit(1)
if not isinstance(data, dict) or not data.get("tag_name"):
    sys.stderr.write("GitHub API returned HTTP 200 without a tag_name field.\n")
    raise SystemExit(1)
print(data["tag_name"])
PY
      parse_ec=$?
      set -e
      rm -f "$body"
      return "$parse_ec"
    fi
    msg="$(github_api_error_text "$body")"
    rm -f "$body"
    if [[ "$code" == "403" || "$code" =~ ^5[0-9][0-9]$ ]]; then
      echo "GitHub API HTTP ${code} (attempt ${attempt}/${GITHUB_API_ATTEMPTS}): ${msg}" >&2
      if [[ "$attempt" -eq "$GITHUB_API_ATTEMPTS" ]]; then
        echo "GitHub API request failed after ${GITHUB_API_ATTEMPTS} attempts (HTTP ${code}): ${msg}" >&2
        return 1
      fi
      sleep "$GITHUB_API_RETRY_DELAY"
      attempt=$((attempt + 1))
      continue
    fi
    echo "GitHub API request failed (HTTP ${code}): ${msg}" >&2
    return 1
  done
  echo "GitHub API request failed after ${GITHUB_API_ATTEMPTS} attempts." >&2
  return 1
}

main() {
  local PINNED EXPECTED LATEST TMP GOT
  PINNED="$(tr -d '[:space:]' < "$ROOT/internal/proxyconf/upstream/VERSION")"
  EXPECTED="$(python3 -c 'import re,sys; text=open(sys.argv[1]).read(); m=re.search(r"GeneratedExampleSHA256\s*=\s*\"([0-9a-f]+)\"", text); print(m.group(1) if m else "")' "$ROOT/internal/proxyconf/catalog_gen.go")"
  LATEST="$(fetch_latest_release_tag)"
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
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
