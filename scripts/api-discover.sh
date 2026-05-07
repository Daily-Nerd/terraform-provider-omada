#!/usr/bin/env bash
# Omada controller API discovery helper.
#
# Logs in, captures the session cookie + omadacId + token, then dumps
# resource payloads so we can identify field names the provider is
# missing (e.g., LAN interface binding for omada_network).
#
# URL pattern (Omada v6.x): {base}/{omadacId}/api/v2/{path}?token={token}
#
# Usage:
#   export OMADA_URL='https://...'
#   export OMADA_USERNAME='...'
#   export OMADA_PASSWORD='...'
#   export OMADA_SITE='Home'
#   ./scripts/api-discover.sh                  # dump networks (default)
#   ./scripts/api-discover.sh networks
#   ./scripts/api-discover.sh devices
#   ./scripts/api-discover.sh dhcp
#   ./scripts/api-discover.sh all              # everything
#   ./scripts/api-discover.sh --keep-cookies   # don't delete cookie jar after
#
# Output written to dist/api-discover/<endpoint>.json

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/api-discover"
COOKIE_JAR="$(mktemp -t omada-cookies.XXXXXX)"
KEEP_COOKIES=0
TARGET="networks"

for arg in "$@"; do
    case "$arg" in
        --keep-cookies) KEEP_COOKIES=1 ;;
        all|networks|devices|dhcp|sites|gateway|wlan|firewall) TARGET="$arg" ;;
        *) echo "error: unknown arg '$arg'" >&2; exit 2 ;;
    esac
done

cleanup() {
    if [[ "$KEEP_COOKIES" == "1" ]]; then
        echo "Cookie jar preserved at: $COOKIE_JAR" >&2
    else
        rm -f "$COOKIE_JAR"
    fi
}
trap cleanup EXIT

mkdir -p "$OUT_DIR"

command -v jq   >/dev/null || { echo "error: jq not in PATH" >&2; exit 1; }
command -v curl >/dev/null || { echo "error: curl not in PATH" >&2; exit 1; }

CURL=(curl -sk -H "Accept: application/json")

# ---------------- omadacId (pre-login, public endpoint) ----------------
INFO=$("${CURL[@]}" "$OMADA_URL/api/info")
OMADAC_ID=$(echo "$INFO" | jq -r '.result.omadacId')
CONTROLLER_VER=$(echo "$INFO" | jq -r '.result.controllerVer')
[[ -z "$OMADAC_ID" || "$OMADAC_ID" == "null" ]] && { echo "error: failed to fetch omadacId from /api/info" >&2; exit 1; }
echo "→ omadacId=$OMADAC_ID  controllerVer=$CONTROLLER_VER" >&2

# ---------------- login ----------------
echo "→ logging in to $OMADA_URL ..." >&2
LOGIN_BODY=$(jq -nc --arg u "$OMADA_USERNAME" --arg p "$OMADA_PASSWORD" '{username: $u, password: $p}')
LOGIN_RESP=$("${CURL[@]}" -c "$COOKIE_JAR" -X POST \
    "$OMADA_URL/$OMADAC_ID/api/v2/login" \
    -H 'Content-Type: application/json' \
    -d "$LOGIN_BODY")
LOGIN_CODE=$(echo "$LOGIN_RESP" | jq -r '.errorCode')
if [[ "$LOGIN_CODE" != "0" ]]; then
    echo "$LOGIN_RESP" | jq '.'
    echo "login failed" >&2
    exit 1
fi
TOKEN=$(echo "$LOGIN_RESP" | jq -r '.result.token')
[[ -z "$TOKEN" || "$TOKEN" == "null" ]] && { echo "error: no token in login response" >&2; exit 1; }
echo "→ token captured" >&2

# CSRF header required for all subsequent requests
CURL+=(-H "Csrf-Token: $TOKEN")

# Helper: build URL with token (handles paths that already contain ?query)
url() {
    local path="$1"
    local sep="?"
    [[ "$path" == *"?"* ]] && sep="&"
    echo "$OMADA_URL/$OMADAC_ID/api/v2${path}${sep}token=$TOKEN"
}

# ---------------- resolve site_id from name ----------------
SITES_RAW=$("${CURL[@]}" -b "$COOKIE_JAR" "$(url "/sites?currentPage=1&currentPageSize=100")")
echo "$SITES_RAW" > "${OUT_DIR}/sites-list.json"
SITE_ID=$(echo "$SITES_RAW" | jq -r --arg name "$OMADA_SITE" '
    .result.data[] | select(.name == $name) | .id
' | head -1)
if [[ -z "$SITE_ID" || "$SITE_ID" == "null" ]]; then
    echo "site '$OMADA_SITE' not found. Available:" >&2
    echo "$SITES_RAW" | jq -r '.result.data[]? | "  - \(.name) (id: \(.id))"' >&2
    echo "Raw: $(echo "$SITES_RAW" | jq '{errorCode, msg}')" >&2
    exit 1
fi
echo "→ siteId=$SITE_ID" >&2
echo

# ---------------- fetch helper ----------------
fetch() {
    local label="$1"
    local path="$2"
    local outfile="${OUT_DIR}/${label}.json"
    local full
    full=$(url "$path")
    echo "→ GET $path" >&2
    local resp
    resp=$("${CURL[@]}" -b "$COOKIE_JAR" "$full")
    echo "$resp" > "$outfile"
    local code
    code=$(echo "$resp" | jq -r '.errorCode // "?"')
    local size
    size=$(wc -c < "$outfile" | tr -d ' ')
    if [[ "$code" == "0" ]]; then
        echo "  ✓ wrote $outfile (${size} bytes)" >&2
    else
        echo "  ! errorCode=$code  msg=$(echo "$resp" | jq -r '.msg // ""')" >&2
        echo "    file: $outfile" >&2
    fi
}

# ---------------- discovery functions ----------------
discover_networks() {
    fetch "networks-lan" "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100"
    # Fetch detail for each network found
    local list_file="${OUT_DIR}/networks-lan.json"
    local nids
    nids=$(jq -r '.result.data[]?.id // empty' "$list_file" 2>/dev/null)
    for nid in $nids; do
        fetch "network-detail-$nid" "/sites/$SITE_ID/setting/lan/networks/$nid"
    done
}

discover_devices() {
    fetch "devices-list" "/sites/$SITE_ID/devices"
}

discover_gateway() {
    # No /gateways endpoint exposed in provider client. Devices list filters by type.
    local devs
    devs=$("${CURL[@]}" -b "$COOKIE_JAR" "$(url "/sites/$SITE_ID/devices")")
    echo "$devs" > "${OUT_DIR}/devices-all.json"
    local gw_mac
    gw_mac=$(echo "$devs" | jq -r '.result[]? | select(.type == "gateway") | .mac' | head -1)
    if [[ -n "$gw_mac" ]]; then
        echo "  gateway mac=$gw_mac" >&2
        # Try several candidate endpoints — record what works
        fetch "gateway-portconf" "/sites/$SITE_ID/setting/portConf/$gw_mac"
        fetch "gateway-routing" "/sites/$SITE_ID/setting/routing"
    else
        echo "  no gateway adopted (limits LAN interface discovery — but the network detail above will still show what fields exist on a created network)" >&2
    fi
}

discover_dhcp() {
    # DHCP reservation endpoint is not in provider client; trying common paths
    fetch "dhcp-reservations-v1" "/sites/$SITE_ID/setting/service/dhcp/reservations?currentPage=1&currentPageSize=100"
    fetch "dhcp-leases-v1" "/sites/$SITE_ID/insight/clients/dhcp?currentPage=1&currentPageSize=100"
}

discover_wlan() {
    fetch "wlan-groups" "/sites/$SITE_ID/setting/wlans"
    fetch "ssids-list" "/sites/$SITE_ID/setting/wlans/all/ssids"
}

discover_firewall() {
    fetch "firewall-acls" "/sites/$SITE_ID/setting/firewall/acls?currentPage=1&currentPageSize=100"
    fetch "ip-groups" "/sites/$SITE_ID/setting/firewall/ipGroups?currentPage=1&currentPageSize=100"
}

discover_sites_only() {
    echo "  ✓ wrote ${OUT_DIR}/sites-list.json" >&2
}

# ---------------- run ----------------
case "$TARGET" in
    networks) discover_networks ;;
    devices)  discover_devices ;;
    gateway)  discover_gateway ;;
    dhcp)     discover_dhcp ;;
    wlan)     discover_wlan ;;
    firewall) discover_firewall ;;
    sites)    discover_sites_only ;;
    all)
        discover_sites_only
        discover_networks
        discover_devices
        discover_gateway
        discover_dhcp
        discover_wlan
        discover_firewall
        ;;
esac

echo
echo "Output dir: $OUT_DIR" >&2
ls -la "$OUT_DIR" >&2 || true
