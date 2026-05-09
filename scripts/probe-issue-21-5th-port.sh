#!/usr/bin/env bash
# Issue #21 probe: find the source of the 5th LAN port ID.
#
# Background: omada_gateway_ports returns 4 ports from /setting/wan/networks ->
# osgPortInfo.wanLanPortSettings, but networks created without explicit
# lan_interface_ids auto-populate with 5 IDs. The 5th ID
# (5_51320fdf135a4ae9b6fddf7fb692e961 in early evidence) is not surfaced by
# the data source.
#
# This script logs in, discovers the 5th port ID empirically by reading a
# real network detail, then probes a battery of endpoints to find where
# (if anywhere) that UUID appears so we can wire the data source against it.
#
# Usage:
#   export OMADA_URL='https://192.168.68.136'
#   export OMADA_USERNAME='Kibukx'
#   export OMADA_PASSWORD='...'
#   export OMADA_SITE='Yggdrasil'
#   ./scripts/probe-issue-21-5th-port.sh

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/issue-21"
mkdir -p "$OUT_DIR"

JAR=$(mktemp -t omada-issue21.XXXXXX)
trap 'rm -f "$JAR"' EXIT

CURL=(curl -sk -H "Accept: application/json")

# ---- 1. omadacId + login ----
echo "[1/6] Fetching omadacId" >&2
OMADAC_ID=$("${CURL[@]}" "$OMADA_URL/api/info" | jq -r '.result.omadacId')
[[ -n "$OMADAC_ID" && "$OMADAC_ID" != "null" ]] || { echo "FAIL: no omadacId" >&2; exit 1; }

echo "[2/6] Logging in" >&2
TOKEN=$("${CURL[@]}" -c "$JAR" -X POST "$OMADA_URL/$OMADAC_ID/api/v2/login" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg u "$OMADA_USERNAME" --arg p "$OMADA_PASSWORD" '{username:$u,password:$p}')" \
    | jq -r '.result.token')
[[ -n "$TOKEN" && "$TOKEN" != "null" ]] || { echo "FAIL: no token" >&2; exit 1; }

CURL+=(-H "Csrf-Token: $TOKEN" -b "$JAR")

api() {
    local path="$1"
    local sep="?"
    [[ "$path" == *"?"* ]] && sep="&"
    echo "$OMADA_URL/$OMADAC_ID/api/v2${path}${sep}token=$TOKEN"
}

# ---- 3. Resolve site ----
echo "[3/6] Resolving site '$OMADA_SITE'" >&2
SITE_ID=$("${CURL[@]}" "$(api "/sites?currentPage=1&currentPageSize=100")" \
    | jq -r --arg n "$OMADA_SITE" '.result.data[] | select(.name==$n).id')
[[ -n "$SITE_ID" && "$SITE_ID" != "null" ]] || { echo "FAIL: site not found" >&2; exit 1; }
echo "  siteId=$SITE_ID" >&2

# ---- 4. Discover 5th port empirically ----
echo "[4/6] Reading interfaceIds from a real network" >&2
"${CURL[@]}" "$(api "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")" \
    > "$OUT_DIR/networks.json"

# Extract all unique interfaceIds across networks
ALL_IDS=$(jq -r '.result.data[]?.interfaceIds[]?' "$OUT_DIR/networks.json" | sort -u)
echo "All interfaceIds across networks:" >&2
echo "$ALL_IDS" | sed 's/^/  /' >&2

# Get gateway port IDs from /setting/wan/networks
"${CURL[@]}" "$(api "/sites/$SITE_ID/setting/wan/networks")" \
    > "$OUT_DIR/wan-networks.json"
WAN_IDS=$(jq -r '.result.osgPortInfo.wanLanPortSettings[]?.portUuid' "$OUT_DIR/wan-networks.json" 2>/dev/null | sort -u)
echo "wanLanPortSettings IDs:" >&2
echo "$WAN_IDS" | sed 's/^/  /' >&2

# Compute the 5th port — IDs in network but not in wan-networks
EXTRA_IDS=$(comm -23 <(echo "$ALL_IDS") <(echo "$WAN_IDS"))
echo "EXTRA IDs (in network.interfaceIds but NOT in wan-networks):" >&2
if [[ -z "$EXTRA_IDS" ]]; then
    echo "  (none — issue may already be resolved or no networks exist)" >&2
    exit 0
fi
echo "$EXTRA_IDS" | sed 's/^/  /' >&2

# Pick the first extra ID for downstream probes
TARGET_ID=$(echo "$EXTRA_IDS" | head -1)
echo "  targetId=$TARGET_ID" >&2

# ---- 5. Probe candidate endpoints ----
echo "[5/6] Probing candidate endpoints for target ID" >&2

# List all gateway devices (typically only 1)
"${CURL[@]}" "$(api "/sites/$SITE_ID/devices")" > "$OUT_DIR/devices.json"
GW_MAC=$(jq -r '.result[]? | select(.type=="gateway" or .type=="0") | .mac' "$OUT_DIR/devices.json" | head -1)
echo "  gateway mac: ${GW_MAC:-<none adopted>}" >&2

probe() {
    local label="$1" path="$2"
    local out="$OUT_DIR/probe-$label.json"
    local resp
    resp=$("${CURL[@]}" "$(api "$path")")
    echo "$resp" > "$out"
    local code=$(echo "$resp" | jq -r '.errorCode // "?"')
    local hits=$(echo "$resp" | grep -c "$TARGET_ID" || true)
    echo "  $label: errorCode=$code, target_hits=$hits ($out)" >&2
}

probe "wan-networks" "/sites/$SITE_ID/setting/wan/networks"
probe "lan-networks" "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100"
probe "site-setting" "/sites/$SITE_ID/setting"

if [[ -n "$GW_MAC" ]]; then
    probe "gw-portConf"     "/sites/$SITE_ID/setting/portConf/$GW_MAC"
    probe "gw-detail"       "/sites/$SITE_ID/devices/$GW_MAC"
    probe "gw-ports"        "/sites/$SITE_ID/devices/$GW_MAC/ports?currentPage=1&currentPageSize=100"
    probe "gw-portsetting"  "/sites/$SITE_ID/devices/$GW_MAC/portSetting"
    probe "gw-routing"      "/sites/$SITE_ID/setting/routing"
    probe "gw-vpn"          "/sites/$SITE_ID/setting/vpn"
    probe "gw-lag"          "/sites/$SITE_ID/setting/lag"
fi

# Search the full payloads for any field name colocated with the target ID
echo "[6/6] Locating target ID in payloads" >&2
for f in "$OUT_DIR"/*.json; do
    if grep -q "$TARGET_ID" "$f" 2>/dev/null; then
        echo "  FOUND in: $(basename "$f")" >&2
        # Extract the JSON path to the matching value
        jq --arg id "$TARGET_ID" '
            paths(strings)
            | . as $path
            | select(getpath($path) == $id)
            | $path | join(".")
        ' "$f" 2>/dev/null | sed 's/^/    path: /' >&2 || true
    fi
done

echo
echo "Output: $OUT_DIR" >&2
echo "Target ID to investigate: $TARGET_ID" >&2
echo
echo "Next: inspect $OUT_DIR/probe-*.json for the JSON path containing the target ID." >&2
echo "That path identifies which Omada concept the 5th port represents." >&2
