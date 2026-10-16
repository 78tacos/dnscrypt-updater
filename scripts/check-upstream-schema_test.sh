#!/usr/bin/env bash
# Exercises the GitHub releases request: auth header, 403/5xx retries,
# and a clear failure instead of a JSON decode traceback.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=check-upstream-schema.sh
source "$ROOT/scripts/check-upstream-schema.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

install_curl() {
  local script="$1"
  mkdir -p "$WORKDIR/bin"
  cat >"$WORKDIR/bin/curl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
$script
EOF
  chmod +x "$WORKDIR/bin/curl"
}

run_fetch() {
  PATH="$WORKDIR/bin:$PATH" \
    GITHUB_API_RETRY_DELAY=0 \
    GITHUB_API_ATTEMPTS=3 \
    fetch_latest_release_tag
}

# --- authenticated 200 ---
install_curl '
log="${CURL_LOG:?}"
out=""
fmt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -w) fmt="$2"; shift 2 ;;
    -H) printf "%s\n" "$2" >> "$log"; shift 2 ;;
    -sS|-s|-S) shift ;;
    *) shift ;;
  esac
done
printf "%s\n" "{\"tag_name\":\"2.1.18\"}" > "$out"
printf "%s" "200"
'
export CURL_LOG="$WORKDIR/headers.txt"
: >"$CURL_LOG"
tag="$(GITHUB_TOKEN=test-token run_fetch)"
[[ "$tag" == "2.1.18" ]] || fail "expected tag 2.1.18, got ${tag}"
grep -qx "Authorization: Bearer test-token" "$CURL_LOG" || fail "missing bearer token header"
grep -qx "Accept: application/vnd.github+json" "$CURL_LOG" || fail "missing accept header"

# --- no token: do not send an empty bearer ---
: >"$CURL_LOG"
tag="$(unset GITHUB_TOKEN; run_fetch)"
[[ "$tag" == "2.1.18" ]] || fail "untokened fetch tag"
if grep -q "Authorization:" "$CURL_LOG"; then
  fail "sent Authorization without GITHUB_TOKEN"
fi

# --- 403 then 500 then 200 ---
install_curl '
state="${CURL_STATE:?}"
log="${CURL_LOG:?}"
n=0
[[ -f "$state" ]] && n="$(cat "$state")"
n=$((n + 1))
printf "%s" "$n" > "$state"
out=""
fmt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -w) fmt="$2"; shift 2 ;;
    -H) printf "%s\n" "$2" >> "$log"; shift 2 ;;
    -sS|-s|-S) shift ;;
    *) shift ;;
  esac
done
case "$n" in
  1) code=403; body="{\"message\":\"API rate limit exceeded\"}" ;;
  2) code=502; body="{\"message\":\"Server Error\"}" ;;
  *) code=200; body="{\"tag_name\":\"2.1.18\"}" ;;
esac
printf "%s" "$body" > "$out"
printf "%s" "$code"
'
export CURL_STATE="$WORKDIR/state"
rm -f "$CURL_STATE"
: >"$CURL_LOG"
err="$WORKDIR/retry.err"
tag="$(GITHUB_TOKEN=test-token run_fetch 2>"$err")"
[[ "$tag" == "2.1.18" ]] || fail "retry path tag, got ${tag}"
grep -q "HTTP 403" "$err" || fail "expected HTTP 403 notice"
grep -q "HTTP 502" "$err" || fail "expected HTTP 502 notice"
[[ "$(grep -c "Authorization: Bearer test-token" "$CURL_LOG")" -eq 3 ]] || fail "expected bearer on each attempt"

# --- 403 exhausted: clear message, no JSONDecodeError ---
install_curl '
log="${CURL_LOG:?}"
out=""
fmt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -w) fmt="$2"; shift 2 ;;
    -H) printf "%s\n" "$2" >> "$log"; shift 2 ;;
    -sS|-s|-S) shift ;;
    *) shift ;;
  esac
done
printf "%s" "{\"message\":\"API rate limit exceeded for 1.2.3.4\"}" > "$out"
printf "%s" "403"
'
: >"$CURL_LOG"
err="$WORKDIR/fail403.err"
set +e
tag="$(GITHUB_TOKEN=test-token run_fetch 2>"$err")"
ec=$?
set -e
[[ "$ec" -ne 0 ]] || fail "403 storm should fail"
[[ -z "$tag" ]] || fail "403 storm should not print a tag"
grep -q "JSONDecodeError" "$err" && fail "JSONDecodeError leaked into the error"
grep -q "failed after 3 attempts (HTTP 403)" "$err" || fail "missing clear 403 failure: $(cat "$err")"
grep -q "API rate limit exceeded for 1.2.3.4" "$err" || fail "missing GitHub message"

# --- 500 exhausted ---
install_curl '
out=""
fmt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -w) fmt="$2"; shift 2 ;;
    -H) shift 2 ;;
    -sS|-s|-S) shift ;;
    *) shift ;;
  esac
done
printf "%s" "bad gateway" > "$out"
printf "%s" "500"
'
err="$WORKDIR/fail500.err"
set +e
tag="$(GITHUB_TOKEN=test-token run_fetch 2>"$err")"
ec=$?
set -e
[[ "$ec" -ne 0 ]] || fail "500 storm should fail"
grep -q "JSONDecodeError" "$err" && fail "JSONDecodeError leaked on 500"
grep -q "failed after 3 attempts (HTTP 500)" "$err" || fail "missing clear 500 failure: $(cat "$err")"
grep -q "bad gateway" "$err" || fail "missing plain-text body"

# --- non-JSON 200 is a clear error, not a traceback ---
install_curl '
out=""
fmt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -w) fmt="$2"; shift 2 ;;
    -H) shift 2 ;;
    -sS|-s|-S) shift ;;
    *) shift ;;
  esac
done
printf "%s" "not-json" > "$out"
printf "%s" "200"
'
err="$WORKDIR/badjson.err"
set +e
tag="$(GITHUB_TOKEN=test-token run_fetch 2>"$err")"
ec=$?
set -e
[[ "$ec" -ne 0 ]] || fail "bad json should fail"
grep -q "JSONDecodeError" "$err" && fail "traceback leaked"
grep -q "was not JSON" "$err" || fail "missing clear JSON message: $(cat "$err")"

echo "schema drift API checks passed"
