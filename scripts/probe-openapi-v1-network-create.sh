#!/usr/bin/env bash
# Probe the /openapi/v1/.../networks/confirm endpoint — the v6 API path
# for creating L3 (interface-type) networks with the ER707 as DHCP server.
#
# Discovery from browser dev tools: the OC200 UI uses this endpoint when
# "DHCP Server Device = Main Router" is selected, NOT the legacy
# /api/v2/setting/lan/networks path we've been probing.
#
# Usage:
#   source ~/.config/homelab/omada.env
#   ./scripts/probe-openapi-v1-network-create.sh

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/probe-openapi-v1"
mkdir -p "$OUT_DIR"

JAR=$(mktemp -t omada-openapi.XXXXXX)
trap 'rm -f "$JAR"' EXIT

CURL=(curl -sk -H "Accept: application/json")

echo "[1] Login" >&2
OMADAC_ID=$("${CURL[@]}" "$OMADA_URL/api/info" | jq -r '.result.omadacId')
TOKEN=$("${CURL[@]}" -c "$JAR" -X POST "$OMADA_URL/$OMADAC_ID/api/v2/login" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg u "$OMADA_USERNAME" --arg p "$OMADA_PASSWORD" '{username:$u,password:$p}')" \
    | jq -r '.result.token')
[[ -n "$TOKEN" && "$TOKEN" != "null" ]] || { echo "FAIL: no token" >&2; exit 1; }
CURL+=(-H "Csrf-Token: $TOKEN" -b "$JAR")

api_v2() {
    local path="$1"
    local sep="?"
    [[ "$path" == *"?"* ]] && sep="&"
    echo "$OMADA_URL/$OMADAC_ID/api/v2${path}${sep}token=$TOKEN"
}
# openapi/v1 does NOT use the token query param. Auth via session cookie
# + Csrf-Token header + Omada-Request-Source: web-local.
api_v1() { echo "$OMADA_URL/openapi/v1/$OMADAC_ID${1}"; }

# Separate curl invocation for openapi/v1 — adds the web-local header the
# browser session uses. The /api/v2 token query param is omitted.
CURL_V1=(curl -sk \
    -H "Accept: application/json" \
    -H "Csrf-Token: $TOKEN" \
    -H "Omada-Request-Source: web-local" \
    -H "X-Requested-With: XMLHttpRequest" \
    -b "$JAR")

SITE_ID=$("${CURL[@]}" "$(api_v2 "/sites?currentPage=1&currentPageSize=100")" \
    | jq -r --arg n "$OMADA_SITE" '.result.data[] | select(.name==$n).id')
echo "  siteId=$SITE_ID omadacId=$OMADAC_ID" >&2

# ER707 MAC (captured from browser dev tools)
GW_MAC="AC-A7-F1-12-0C-6B"
PORT2="2_2b95b4f331d6443da942b0f6b24ef4c5"

# Probe Q: POST openapi/v1/.../networks/confirm — structure matches the
# browser EXACTLY. Key insight from second capture: deviceList, tagIds,
# portIsolationEnable, flowControlEnable are all nested INSIDE deviceConfig,
# not at top level. Single port selection works when body is correctly shaped.
CONFIRM_BODY=$(jq -nc \
    --arg mac "$GW_MAC" \
    --arg p2 "$PORT2" '{
    deviceConfig: {
        portIsolationEnable: false,
        flowControlEnable: false,
        deviceList: [{
            mac: $mac,
            type: 1,
            ports: [$p2],
            lags: []
        }],
        tagIds: []
    },
    lanNetwork: {
        name: "probe-q-openapi",
        deviceMac: $mac,
        deviceType: 1,
        vlanType: 0,
        vlan: 97,
        gatewaySubnet: "10.10.97.1/24",
        dhcpSettings: {
            enable: true,
            ipRangePool: [{ipaddrStart: "10.10.97.100", ipaddrEnd: "10.10.97.250"}],
            dhcpns: "auto",
            leasetime: 120,
            gatewayMode: "auto",
            options: []
        },
        upnpLanEnable: false,
        igmpSnoopEnable: false,
        dhcpGuard: {enable: false},
        dhcpv6Guard: {enable: false},
        lanNetworkIpv6Config: {proto: 0, enable: 0},
        qosQueueEnable: false,
        isolation: false,
        mldSnoopEnable: false,
        arpDetectionEnable: false,
        dhcpL2RelayEnable: false
    }
}')
echo "$CONFIRM_BODY" > "$OUT_DIR/probe-q-body.json"

echo "[Q] POST openapi/v1/sites/$SITE_ID/networks/confirm (port2 only)" >&2
Q_RESP=$("${CURL_V1[@]}" \
    -H 'Content-Type: application/json' \
    -X POST "$(api_v1 "/sites/$SITE_ID/networks/confirm")" \
    -d "$CONFIRM_BODY")
echo "$Q_RESP" | jq '.' | tee "$OUT_DIR/probe-q-response.json" >&2

NEW_ID=$(echo "$Q_RESP" | jq -r '
    if .errorCode == 0 then
        (.result.networkIdList[0] // empty)
    else empty end' 2>/dev/null || true)

if [[ -n "$NEW_ID" ]]; then
    echo "  [Q] SUCCESS: id=$NEW_ID" >&2

    # Read back via api/v2 list — check purpose, gatewaySubnet, dhcpSettings
    echo "  [Q] readback via api/v2 list:" >&2
    "${CURL[@]}" "$(api_v2 "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")" \
        | jq --arg id "$NEW_ID" '
            .result.data[] | select(.id==$id) |
            {id, name, purpose, vlan, gatewaySubnet, dhcpSettings, interfaceIds, allLan, interface, deviceType}' \
        | tee "$OUT_DIR/probe-q-readback.json" >&2

    # Also try readback via openapi/v1 list (may be different)
    echo "  [Q] readback via openapi/v1 list:" >&2
    "${CURL_V1[@]}" "$(api_v1 "/sites/$SITE_ID/networks?currentPage=1&currentPageSize=100")" \
        | jq --arg id "$NEW_ID" '
            if .result.data then
                .result.data[] | select(.id==$id)
            else . end' \
        | tee "$OUT_DIR/probe-q-readback-v1.json" >&2 || true

    # Cleanup via api/v2 DELETE
    echo "  [cleanup] DELETE $NEW_ID" >&2
    "${CURL[@]}" -X DELETE "$(api_v2 "/sites/$SITE_ID/setting/lan/networks/$NEW_ID")" \
        | jq '{errorCode, msg}' >&2
else
    echo "  [Q] FAILED or no ID in response" >&2
fi

echo "" >&2
echo "=== SUMMARY ===" >&2
echo "Probe Q body:     $OUT_DIR/probe-q-body.json" >&2
echo "Probe Q response: $OUT_DIR/probe-q-response.json" >&2
echo "Readback v2:      $OUT_DIR/probe-q-readback.json" >&2
echo "Readback v1:      $OUT_DIR/probe-q-readback-v1.json" >&2
