#!/usr/bin/env bash
# Compatible with bash 3.2 (macOS default).
# Investigation: -1001 "Invalid request parameters" when PATCHing a network
# from purpose=vlan to purpose=interface.
#
# This script:
#   1. Logs in and resolves site_id
#   2. Lists all networks, dumps each detail (including the "Default" network
#      which is already purpose=interface and known-good)
#   3. Builds a minimal PATCH body matching what our provider currently sends
#      and applies it against ONE of our existing networks (cameras = VID 60)
#   4. Captures full response so we can see what field controller objected to
#   5. Tries the same PATCH with leasetime + dhcpns added (the strongest
#      hypothesis for the -1001 failure)
#
# Output: dist/issue-network-update-1001/*.json + summary
#
# Usage:
#   source ~/.config/homelab/omada.env
#   ./scripts/probe-network-update-1001.sh

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/issue-network-update-1001"
mkdir -p "$OUT_DIR"

JAR=$(mktemp -t omada-net1001.XXXXXX)
trap 'rm -f "$JAR"' EXIT

CURL=(curl -sk -H "Accept: application/json")

echo "[1/6] Login" >&2
OMADAC_ID=$("${CURL[@]}" "$OMADA_URL/api/info" | jq -r '.result.omadacId')
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

SITE_ID=$("${CURL[@]}" "$(api "/sites?currentPage=1&currentPageSize=100")" \
    | jq -r --arg n "$OMADA_SITE" '.result.data[] | select(.name==$n).id')
echo "  siteId=$SITE_ID" >&2

echo "[2/6] Listing networks" >&2
NET_LIST=$("${CURL[@]}" "$(api "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")")
echo "$NET_LIST" > "$OUT_DIR/networks-list.json"

echo "  detail per network:" >&2
echo "$NET_LIST" | jq -r '.result.data[] | "\(.id) \(.name) \(.purpose // "?") vlan=\(.vlan)"' >&2

echo "[3/6] Dumping each network's full JSON (find any pre-existing interface-purpose nets)" >&2
echo "$NET_LIST" | jq -r '.result.data[].id' | while read -r nid; do
    "${CURL[@]}" "$(api "/sites/$SITE_ID/setting/lan/networks/$nid")" \
        > "$OUT_DIR/network-detail-${nid}.json"
done

echo "[4/6] Locating the cameras network for the probe" >&2
CAMERAS_ID=$(echo "$NET_LIST" | jq -r '.result.data[] | select(.name=="cameras").id')
echo "  cameras_id=$CAMERAS_ID" >&2

# Pick the ER707 LAN port 2 UUID (matches what TF sends)
PORT2_ID="2_2b95b4f331d6443da942b0f6b24ef4c5"

# Reproduce the provider's PATCH body (no leasetime, no dhcpns)
PATCH_MIN=$(jq -nc --arg gw "10.10.60.1/24" --arg p2 "$PORT2_ID" '{
    name:"cameras",
    purpose:"interface",
    vlan:60,
    gatewaySubnet:$gw,
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.60.100",
        ipaddrEnd:"10.10.60.250"
    },
    interfaceIds:[$p2],
    isolation:false,
    igmpSnoopEnable:false,
    application:0,
    vlanType:0,
    fastLeaveEnable:false,
    mldSnoopEnable:false,
    dhcpv6Guard:{enable:false},
    dhcpGuard:{enable:false},
    dhcpL2RelayEnable:false,
    portal:false,
    accessControlRule:false,
    rateLimit:false,
    arpDetectionEnable:false
}')
echo "$PATCH_MIN" > "$OUT_DIR/patch-min-body.json"

echo "[5/6] Probe A: PATCH cameras with minimal body (provider current)" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_MIN" \
    > "$OUT_DIR/patch-min-response.json"
jq '.' "$OUT_DIR/patch-min-response.json" >&2

# Add leasetime + dhcpns
PATCH_FULL=$(echo "$PATCH_MIN" | jq '.dhcpSettings.leasetime = 14400 | .dhcpSettings.dhcpns = "auto"')
echo "$PATCH_FULL" > "$OUT_DIR/patch-full-body.json"

echo "[6/6] Probe B: PATCH cameras with leasetime + dhcpns added" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_FULL" \
    > "$OUT_DIR/patch-full-response.json"
jq '.' "$OUT_DIR/patch-full-response.json" >&2

echo "" >&2
echo "=== SUMMARY ===" >&2
echo "Network list: $OUT_DIR/networks-list.json" >&2
echo "Per-network detail: $OUT_DIR/network-detail-*.json" >&2
echo "Probe A (minimal):   $OUT_DIR/patch-min-response.json" >&2
echo "Probe B (with DHCP defaults): $OUT_DIR/patch-full-response.json" >&2
