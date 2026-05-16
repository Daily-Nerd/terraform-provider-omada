#!/usr/bin/env bash
# Probe POST /openapi/v1/{omadacId}/sites/{siteId}/networks/{id}/check
# to figure out what body shape the controller accepts on update.
#
# Provider currently fails with "API error -1001: must not be null" — no
# field name given. This script bisects: starts from the real /api/v2
# GetNetwork response (which has every field the controller knows about),
# adds the openapi/v1 dhcp tweaks, and probes /check with progressively
# trimmed / augmented bodies until -1001 goes away.
#
# Usage:
#   source ~/.config/homelab/omada.env
#   ./scripts/probe-openapi-v1-network-update.sh <network-id>
#
# Default network-id is iot (6a0679def791ed1f34381508). Override via $1.

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

NET_ID="${1:-6a0679def791ed1f34381508}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/probe-openapi-v1-update"
mkdir -p "$OUT_DIR"

JAR=$(mktemp -t omada-update-probe.XXXXXX)
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

CURL_V1=(curl -sk \
    -H "Accept: application/json" \
    -H "Content-Type: application/json" \
    -H "Csrf-Token: $TOKEN" \
    -H "Omada-Request-Source: web-local" \
    -H "X-Requested-With: XMLHttpRequest" \
    -b "$JAR")

api_v2() {
    local path="$1"
    local sep="?"
    [[ "$path" == *"?"* ]] && sep="&"
    echo "$OMADA_URL/$OMADAC_ID/api/v2${path}${sep}token=$TOKEN"
}
api_v1() { echo "$OMADA_URL/openapi/v1/$OMADAC_ID${1}"; }

SITE_ID=$("${CURL[@]}" "$(api_v2 "/sites?currentPage=1&currentPageSize=100")" \
    | jq -r --arg n "$OMADA_SITE" '.result.data[] | select(.name==$n).id')
echo "  siteId=$SITE_ID omadacId=$OMADAC_ID netId=$NET_ID" >&2

echo "[2] List networks via /api/v2 → $OUT_DIR/raw-list.json" >&2
"${CURL[@]}" "$(api_v2 "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")" > "$OUT_DIR/raw-list.json"
echo "  top-level keys: $(jq -r 'keys | join(",")' "$OUT_DIR/raw-list.json" 2>/dev/null || echo none)" >&2
echo "  errorCode: $(jq -r '.errorCode // "absent"' "$OUT_DIR/raw-list.json")" >&2
echo "  msg: $(jq -r '.msg // "absent"' "$OUT_DIR/raw-list.json")" >&2
echo "  result structure:" >&2
jq '.result | if type == "array" then "array len="+(length|tostring) elif type == "object" then "object keys="+(keys|join(",")) else type end' "$OUT_DIR/raw-list.json" >&2

# Try to extract network — handle both .result.data[] (paginated) and .result[] (bare array) shapes.
jq --arg id "$NET_ID" '
  if (.result | type) == "array" then
    .result[] | select(.id == $id)
  elif (.result.data | type) == "array" then
    .result.data[] | select(.id == $id)
  else
    null
  end
' "$OUT_DIR/raw-list.json" > "$OUT_DIR/current.json"

if [[ ! -s "$OUT_DIR/current.json" ]] || [[ "$(cat "$OUT_DIR/current.json")" == "null" ]]; then
    echo "FAIL: could not extract network $NET_ID — see $OUT_DIR/raw-list.json" >&2
    exit 1
fi
echo "  fields: $(jq -r 'keys | join(",")' "$OUT_DIR/current.json")" >&2
echo "  dhcpSettings keys: $(jq -r '.dhcpSettings | keys | join(",")' "$OUT_DIR/current.json" 2>/dev/null || echo none)" >&2
echo "  fields: $(jq -r 'keys | join(",")' "$OUT_DIR/current.json")" >&2
echo "  dhcpSettings keys: $(jq -r '.dhcpSettings | keys | join(",")' "$OUT_DIR/current.json" 2>/dev/null || echo none)" >&2

probe() {
    local label="$1"
    local body_file="$2"
    local out_file="$OUT_DIR/${label}.response.json"

    echo "--- probe [$label] ---" >&2
    echo "    body: $body_file" >&2

    "${CURL_V1[@]}" -X POST "$(api_v1 "/sites/$SITE_ID/networks/$NET_ID/check")" \
        --data-binary "@$body_file" > "$out_file" || true

    local err msg
    err=$(jq -r '.errorCode // .errcode // "?"' "$out_file" 2>/dev/null || echo "?")
    msg=$(jq -r '.msg // .message // "?"' "$out_file" 2>/dev/null || echo "?")

    if [[ "$err" == "0" ]]; then
        echo "    OK ($err): $msg" >&2
    else
        echo "    FAIL ($err): $msg" >&2
    fi
}

# Build candidate body A: exact GetNetwork result + add dhcpns/priDns to dhcpSettings.
jq '
  . + {
    dhcpSettings: (
      .dhcpSettings + {
        dhcpns: "manual",
        priDns: "192.168.68.97"
      }
    )
  }
' "$OUT_DIR/current.json" > "$OUT_DIR/A_full_passthrough.body.json"

probe "A_full_passthrough" "$OUT_DIR/A_full_passthrough.body.json"

# Candidate B: same but strip purpose (openapi/v1 may not want it on check).
jq 'del(.purpose)' "$OUT_DIR/A_full_passthrough.body.json" > "$OUT_DIR/B_no_purpose.body.json"
probe "B_no_purpose" "$OUT_DIR/B_no_purpose.body.json"

# Candidate C: strip server-computed siteId.
jq 'del(.siteId)' "$OUT_DIR/A_full_passthrough.body.json" > "$OUT_DIR/C_no_siteId.body.json"
probe "C_no_siteId" "$OUT_DIR/C_no_siteId.body.json"

# Candidate D: strip both purpose + siteId + any internal `_id` / mongoId-ish.
jq 'del(.purpose, .siteId, ._id)' "$OUT_DIR/A_full_passthrough.body.json" > "$OUT_DIR/D_minimal_strip.body.json"
probe "D_minimal_strip" "$OUT_DIR/D_minimal_strip.body.json"

# Candidate E: whitelist exactly the fields the OC200 UI sends to /check.
# Adds `upnpLanEnable: false` (GetNetwork doesn't return it) and trims
# every field the UI does NOT echo back (purpose, site, interface,
# interfaceIds, accessControlRule, allLan, exist* flags, origName,
# portal, primary, rateLimit, resource, state, subnetOverride).
jq '
  {
    id,
    name,
    deviceMac,
    deviceType,
    vlanType,
    vlan,
    gatewaySubnet,
    dhcpSettings: {
      enable:       (.dhcpSettings.enable // false),
      ipRangePool:  (.dhcpSettings.ipRangePool // []),
      ipRangeStart: (.dhcpSettings.ipRangeStart // 0),
      ipRangeEnd:   (.dhcpSettings.ipRangeEnd // 0),
      dhcpns:       "manual",
      priDns:       "192.168.68.97",
      leasetime:    (.dhcpSettings.leasetime // 120),
      gatewayMode:  (.dhcpSettings.gatewayMode // "auto"),
      options:      (.dhcpSettings.options // [])
    },
    upnpLanEnable:        false,
    igmpSnoopEnable:      (.igmpSnoopEnable // false),
    dhcpGuard:            (.dhcpGuard // {enable: false}),
    dhcpv6Guard:          (.dhcpv6Guard // {enable: false}),
    lanNetworkIpv6Config: (.lanNetworkIpv6Config // {proto: 0, enable: 0}),
    qosQueueEnable:       (.qosQueueEnable // false),
    isolation:            (.isolation // false),
    mldSnoopEnable:       (.mldSnoopEnable // false),
    arpDetectionEnable:   (.arpDetectionEnable // false),
    dhcpL2RelayEnable:    (.dhcpL2RelayEnable // false),
    application:          (.application // 0),
    fastLeaveEnable:      (.fastLeaveEnable // false),
    existMultiVlan:       (.existMultiVlan // false),
    totalIpNum:           (.totalIpNum // 0),
    dhcpServerNum:        (.dhcpServerNum // 1)
  }
' "$OUT_DIR/current.json" > "$OUT_DIR/E_ui_whitelist.body.json"
probe "E_ui_whitelist" "$OUT_DIR/E_ui_whitelist.body.json"

# Candidate F: literal UI-capture body. Hardcoded exact byte-for-byte
# match of the OC200 paramcheck capture (with the iot-specific values).
# If this passes and E fails, the diff between them is the missing field.
cat > "$OUT_DIR/F_literal_ui_body.json" <<'EOF'
{
  "name": "iot",
  "deviceMac": "AC-A7-F1-12-0C-6B",
  "deviceType": 1,
  "vlanType": 0,
  "vlan": 50,
  "gatewaySubnet": "10.10.50.1/24",
  "dhcpSettings": {
    "enable": true,
    "ipRangePool": [{"ipaddrStart": "10.10.50.100", "ipaddrEnd": "10.10.50.250"}],
    "dhcpns": "manual",
    "priDns": "192.168.68.97",
    "leasetime": 120,
    "gatewayMode": "auto",
    "options": [],
    "ipRangeStart": 168440320,
    "ipRangeEnd": 168440575
  },
  "upnpLanEnable": false,
  "igmpSnoopEnable": false,
  "dhcpGuard": {"enable": false},
  "dhcpv6Guard": {"enable": false},
  "lanNetworkIpv6Config": {"proto": 0, "enable": 0},
  "qosQueueEnable": false,
  "isolation": false,
  "mldSnoopEnable": false,
  "arpDetectionEnable": false,
  "dhcpL2RelayEnable": false,
  "id": "6a0679def791ed1f34381508",
  "application": 0,
  "fastLeaveEnable": false,
  "existMultiVlan": false,
  "totalIpNum": 151,
  "dhcpServerNum": 1
}
EOF
probe "F_literal_ui_body" "$OUT_DIR/F_literal_ui_body.json"

# Candidate G: E body but force proto:0 explicitly (so lanNetworkIpv6Config
# is {proto:0, enable:0}, not the partial {enable:0} GetNetwork returned).
jq '.lanNetworkIpv6Config = {proto: 0, enable: 0}' "$OUT_DIR/E_ui_whitelist.body.json" \
    > "$OUT_DIR/G_proto_fix.body.json"
probe "G_proto_fix" "$OUT_DIR/G_proto_fix.body.json"

echo >&2
echo "=== summary ===" >&2
for f in "$OUT_DIR"/*.response.json; do
    label=$(basename "$f" .response.json)
    err=$(jq -r '.errorCode // .errcode // "?"' "$f" 2>/dev/null || echo "?")
    msg=$(jq -r '.msg // .message // "?"' "$f" 2>/dev/null || echo "?")
    printf "  %-30s err=%s msg=%s\n" "$label" "$err" "$msg" >&2
done
