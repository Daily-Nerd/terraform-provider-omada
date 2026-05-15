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

# Add leasetime + dhcpns. Use 120 (controller's documented default and
# matches the "Default" network in the captured api-discover dump).
# Controller rejects leasetime outside [2, 10080] minutes.
PATCH_FULL=$(echo "$PATCH_MIN" | jq '.dhcpSettings.leasetime = 120 | .dhcpSettings.dhcpns = "auto"')
echo "$PATCH_FULL" > "$OUT_DIR/patch-full-body.json"

echo "[6/8] Probe B: PATCH cameras with leasetime=120 + dhcpns=auto added" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_FULL" \
    > "$OUT_DIR/patch-full-response.json"
jq '.' "$OUT_DIR/patch-full-response.json" >&2

# Probe C: add allLan=false explicitly (current PATCH omits this — controller
# may default to allLan=true and reject explicit interfaceIds binding when
# Default already claims all-LAN-ports).
PATCH_ALLLAN=$(echo "$PATCH_FULL" | jq '. + {allLan: false}')
echo "$PATCH_ALLLAN" > "$OUT_DIR/patch-alllan-body.json"

echo "[7/8] Probe C: same as B + allLan=false explicit" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_ALLLAN" \
    > "$OUT_DIR/patch-alllan-response.json"
jq '.' "$OUT_DIR/patch-alllan-response.json" >&2

# Probe D: add subnetOverride flags (Default network has subnetOverride=true,
# subnetOverrideEnable=false). Speculative — may or may not be required.
PATCH_SUBNET=$(echo "$PATCH_ALLLAN" | jq '. + {subnetOverride: true, subnetOverrideEnable: false}')
echo "$PATCH_SUBNET" > "$OUT_DIR/patch-subnet-body.json"

echo "[8/8] Probe D: + subnetOverride/subnetOverrideEnable" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_SUBNET" \
    > "$OUT_DIR/patch-subnet-response.json"
jq '.' "$OUT_DIR/patch-subnet-response.json" >&2

echo "" >&2
echo "=== SUMMARY ===" >&2
echo "Network list:            $OUT_DIR/networks-list.json" >&2
echo "Per-network detail:      $OUT_DIR/network-detail-*.json" >&2
echo "Probe A (minimal):       $OUT_DIR/patch-min-response.json" >&2
echo "Probe B (+leasetime120): $OUT_DIR/patch-full-response.json" >&2
echo "Probe C (+allLan=false): $OUT_DIR/patch-alllan-response.json" >&2
echo "Probe D (+subnetOverride): $OUT_DIR/patch-subnet-response.json" >&2

# Probe E: minimal body — just purpose + gateway + DHCP. Strip all the
# feature toggles. Bisects whether one of those toggles is what -1
# objects to.
PATCH_MIN_INTERFACE=$(jq -nc --arg p2 "$PORT2_ID" '{
    name:"cameras",
    purpose:"interface",
    vlan:60,
    gatewaySubnet:"10.10.60.1/24",
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.60.100",
        ipaddrEnd:"10.10.60.250",
        leasetime:120,
        dhcpns:"auto"
    },
    interfaceIds:[$p2]
}')
echo "$PATCH_MIN_INTERFACE" > "$OUT_DIR/patch-min-interface-body.json"

echo "[9/10] Probe E: minimal interface body (no feature toggles)" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_MIN_INTERFACE" \
    > "$OUT_DIR/patch-min-interface-response.json"
jq '.' "$OUT_DIR/patch-min-interface-response.json" >&2

# Probe F: just flip purpose, nothing else. See whether ANY interface PATCH
# is accepted.
PATCH_PURPOSE_ONLY=$(jq -nc '{
    name:"cameras",
    purpose:"interface",
    vlan:60
}')
echo "$PATCH_PURPOSE_ONLY" > "$OUT_DIR/patch-purpose-only-body.json"

echo "[10/10] Probe F: purpose-only flip (no gateway, no DHCP, no interfaces)" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_PURPOSE_ONLY" \
    > "$OUT_DIR/patch-purpose-only-response.json"
jq '.' "$OUT_DIR/patch-purpose-only-response.json" >&2

# Probe G: same as B + ipRangePool array. Omada 6.x may require the
# modern multi-range pool shape instead of (or in addition to) the
# legacy ipaddrStart/ipaddrEnd pair.
PATCH_POOL=$(echo "$PATCH_FULL" | jq '
    .dhcpSettings.ipRangePool = [{ipaddrStart: "10.10.60.100", ipaddrEnd: "10.10.60.250"}]
')
echo "$PATCH_POOL" > "$OUT_DIR/patch-pool-body.json"

echo "[11/13] Probe G: + dhcpSettings.ipRangePool array" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_POOL" \
    > "$OUT_DIR/patch-pool-response.json"
jq '.' "$OUT_DIR/patch-pool-response.json" >&2

# Probe H: keep ALL 5 current interfaceIds, just flip purpose and add
# gateway/DHCP. Tests whether the interfaceIds shrink (5 -> 1) is what
# triggers -1, not the purpose transition itself.
PATCH_KEEP_PORTS=$(echo "$PATCH_FULL" | jq '
    .interfaceIds = [
        "2_2b95b4f331d6443da942b0f6b24ef4c5",
        "4_72a10839d1864cbf8861d20182b442fe",
        "5_51320fdf135a4ae9b6fddf7fb692e961",
        "6_ddd1e5921a5e4181b2cd738014ff0d71",
        "7_06fa7e6034984023a69499bb2ad63058"
    ]
')
echo "$PATCH_KEEP_PORTS" > "$OUT_DIR/patch-keep-ports-body.json"

echo "[12/13] Probe H: keep all 5 interfaceIds + purpose flip" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_KEEP_PORTS" \
    > "$OUT_DIR/patch-keep-ports-response.json"
jq '.' "$OUT_DIR/patch-keep-ports-response.json" >&2

# Probe I: stay in purpose=vlan but shrink interfaceIds 5->1. Tests
# whether the controller will accept the interfaceIds change at all
# when purpose isn't flipping.
PATCH_SHRINK_ONLY=$(jq -nc --arg p2 "$PORT2_ID" '{
    name:"cameras",
    purpose:"vlan",
    vlan:60,
    interfaceIds:[$p2],
    igmpSnoopEnable:false
}')
echo "$PATCH_SHRINK_ONLY" > "$OUT_DIR/patch-shrink-only-body.json"

echo "[13/13] Probe I: shrink interfaceIds 5->1, keep purpose=vlan" >&2
"${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$CAMERAS_ID")" \
    -H 'Content-Type: application/json' \
    -d "$PATCH_SHRINK_ONLY" \
    > "$OUT_DIR/patch-shrink-only-response.json"
jq '.' "$OUT_DIR/patch-shrink-only-response.json" >&2

# Probe J: POST a brand-new interface-purpose network (scratch create).
# Tests whether the CREATE endpoint also rejects purpose=interface, which
# is what the provider does after RequiresReplace triggers destroy+create.
# VLAN 99 is unused — safe test slot. Clean up afterwards via Probe K.
POST_INTERFACE=$(jq -nc --arg p2 "$PORT2_ID" '{
    name:"probe-test-interface",
    purpose:"interface",
    vlan:99,
    gatewaySubnet:"10.10.99.1/24",
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.99.100",
        ipaddrEnd:"10.10.99.250",
        leasetime:120,
        dhcpns:"auto"
    },
    interfaceIds:[$p2],
    igmpSnoopEnable:false,
    isolation:false,
    application:0,
    vlanType:0,
    fastLeaveEnable:false,
    mldSnoopEnable:false,
    dhcpL2RelayEnable:false,
    portal:false,
    accessControlRule:false,
    rateLimit:false,
    arpDetectionEnable:false,
    dhcpv6Guard:{enable:false},
    dhcpGuard:{enable:false}
}')
echo "$POST_INTERFACE" > "$OUT_DIR/post-interface-body.json"

echo "[J] Probe J: POST new interface-purpose network (scratch create)" >&2
"${CURL[@]}" -X POST "$(api "/sites/$SITE_ID/setting/lan/networks")" \
    -H 'Content-Type: application/json' \
    -d "$POST_INTERFACE" \
    > "$OUT_DIR/post-interface-response.json"
jq '.' "$OUT_DIR/post-interface-response.json" >&2

# Extract the new network ID if creation succeeded (errorCode=0).
# Omada POST /networks returns .result as a raw string ID, not an object.
NEW_NET_ID=$(jq -r 'if .errorCode == 0 then .result else empty end' "$OUT_DIR/post-interface-response.json" 2>/dev/null || true)

# Probe K: POST with purpose=vlan to see if vlan creation works at all
# (baseline — if this also fails the issue is something else entirely).
POST_VLAN=$(jq -nc '{
    name:"probe-test-vlan",
    purpose:"vlan",
    vlan:98,
    igmpSnoopEnable:false
}')
echo "$POST_VLAN" > "$OUT_DIR/post-vlan-body.json"

echo "[K] Probe K: POST new vlan-purpose network (baseline)" >&2
"${CURL[@]}" -X POST "$(api "/sites/$SITE_ID/setting/lan/networks")" \
    -H 'Content-Type: application/json' \
    -d "$POST_VLAN" \
    > "$OUT_DIR/post-vlan-response.json"
jq '.' "$OUT_DIR/post-vlan-response.json" >&2

NEW_VLAN_ID=$(jq -r 'if .errorCode == 0 then .result else empty end' "$OUT_DIR/post-vlan-response.json" 2>/dev/null || true)

# Probe L: PATCH the vlan network immediately to purpose=interface.
# If K just created it, use that ID. If K failed (e.g. VLAN 98 already exists
# from a previous probe run), fall back to finding probe-test-vlan via list.
if [[ -z "$NEW_VLAN_ID" ]]; then
    NEW_VLAN_ID=$("${CURL[@]}" "$(api "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")" \
        | jq -r '.result.data[] | select(.name=="probe-test-vlan").id')
    [[ -n "$NEW_VLAN_ID" ]] && echo "  [L] fallback: found existing probe-test-vlan id=$NEW_VLAN_ID" >&2
fi

if [[ -n "$NEW_VLAN_ID" ]]; then
    PATCH_VLAN_TO_INTERFACE=$(jq -nc --arg p2 "$PORT2_ID" '{
        name:"probe-test-vlan",
        purpose:"interface",
        vlan:98,
        gatewaySubnet:"10.10.98.1/24",
        dhcpSettings:{
            enable:true,
            ipaddrStart:"10.10.98.100",
            ipaddrEnd:"10.10.98.250",
            leasetime:120,
            dhcpns:"auto"
        },
        interfaceIds:[$p2],
        igmpSnoopEnable:false
    }')
    echo "$PATCH_VLAN_TO_INTERFACE" > "$OUT_DIR/patch-vlan-to-interface-body.json"

    echo "[L] Probe L: PATCH probe-test-vlan from vlan->interface immediately after creation" >&2
    "${CURL[@]}" -X PATCH "$(api "/sites/$SITE_ID/setting/lan/networks/$NEW_VLAN_ID")" \
        -H 'Content-Type: application/json' \
        -d "$PATCH_VLAN_TO_INTERFACE" \
        > "$OUT_DIR/patch-vlan-to-interface-response.json"
    jq '.' "$OUT_DIR/patch-vlan-to-interface-response.json" >&2
else
    echo "[L] Probe L: SKIPPED (vlan POST failed, no ID to patch)" >&2
fi

# Probe M: POST purpose=interface with NO interfaceIds.
# Hypothesis: the Default network (only working interface network) has
# interfaceIds=[] (empty). All our previous probes sent interfaceIds=[port2],
# which may be invalid for interface-purpose (v6 creates a virtual L3
# interface, not tied to a specific LAN port UUID). Omitting interfaceIds
# tests if that is what triggers -1.
POST_IFACE_NO_PORTS=$(jq -nc '{
    name:"probe-m-iface-noports",
    purpose:"interface",
    vlan:97,
    gatewaySubnet:"10.10.97.1/24",
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.97.100",
        ipaddrEnd:"10.10.97.250",
        leasetime:120,
        dhcpns:"auto"
    },
    igmpSnoopEnable:false
}')
echo "$POST_IFACE_NO_PORTS" > "$OUT_DIR/post-m-noports-body.json"

echo "[M] Probe M: POST interface WITHOUT interfaceIds (empty L3 bind)" >&2
"${CURL[@]}" -X POST "$(api "/sites/$SITE_ID/setting/lan/networks")" \
    -H 'Content-Type: application/json' \
    -d "$POST_IFACE_NO_PORTS" \
    > "$OUT_DIR/post-m-noports-response.json"
jq '.' "$OUT_DIR/post-m-noports-response.json" >&2
NEW_M_ID=$(jq -r 'if .errorCode == 0 then .result else empty end' "$OUT_DIR/post-m-noports-response.json" 2>/dev/null || true)

# Probe N: POST purpose=interface with deviceType=1 and NO interfaceIds.
# The Default network has deviceType=1 (vs vlan=0). This may be required
# for the controller to provision the L3 gateway interface.
POST_IFACE_DEVTYPE=$(jq -nc '{
    name:"probe-n-iface-devtype",
    purpose:"interface",
    vlan:96,
    gatewaySubnet:"10.10.96.1/24",
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.96.100",
        ipaddrEnd:"10.10.96.250",
        leasetime:120,
        dhcpns:"auto"
    },
    deviceType:1,
    igmpSnoopEnable:false
}')
echo "$POST_IFACE_DEVTYPE" > "$OUT_DIR/post-n-devtype-body.json"

echo "[N] Probe N: POST interface + deviceType=1 (no interfaceIds)" >&2
"${CURL[@]}" -X POST "$(api "/sites/$SITE_ID/setting/lan/networks")" \
    -H 'Content-Type: application/json' \
    -d "$POST_IFACE_DEVTYPE" \
    > "$OUT_DIR/post-n-devtype-response.json"
jq '.' "$OUT_DIR/post-n-devtype-response.json" >&2
NEW_N_ID=$(jq -r 'if .errorCode == 0 then .result else empty end' "$OUT_DIR/post-n-devtype-response.json" 2>/dev/null || true)

# Probe O: POST purpose=interface with interfaceIds=[] (explicit empty array).
# Tests whether explicit empty array differs from field absence.
POST_IFACE_EMPTY_IDS=$(jq -nc '{
    name:"probe-o-iface-emptyids",
    purpose:"interface",
    vlan:95,
    gatewaySubnet:"10.10.95.1/24",
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.95.100",
        ipaddrEnd:"10.10.95.250",
        leasetime:120,
        dhcpns:"auto"
    },
    interfaceIds:[],
    igmpSnoopEnable:false
}')
echo "$POST_IFACE_EMPTY_IDS" > "$OUT_DIR/post-o-emptyids-body.json"

echo "[O] Probe O: POST interface + interfaceIds=[] (explicit empty array)" >&2
"${CURL[@]}" -X POST "$(api "/sites/$SITE_ID/setting/lan/networks")" \
    -H 'Content-Type: application/json' \
    -d "$POST_IFACE_EMPTY_IDS" \
    > "$OUT_DIR/post-o-emptyids-response.json"
jq '.' "$OUT_DIR/post-o-emptyids-response.json" >&2
NEW_O_ID=$(jq -r 'if .errorCode == 0 then .result else empty end' "$OUT_DIR/post-o-emptyids-response.json" 2>/dev/null || true)

# Probe P: POST purpose=vlan WITH gatewaySubnet + dhcpSettings.
# KEY HYPOTHESIS: In Omada v6, purpose=vlan networks can have L3 settings.
# purpose=interface may be an internal-only designation (Default network).
# The ER707 routes all VLANs when gatewaySubnet+dhcpSettings are set on
# purpose=vlan. Probe I (PATCH cameras vlan+interfaceIds) already succeeded —
# this extends it to also include gateway and DHCP settings in a POST.
POST_VLAN_L3=$(jq -nc --arg p2 "$PORT2_ID" '{
    name:"probe-p-vlan-l3",
    purpose:"vlan",
    vlan:97,
    gatewaySubnet:"10.10.97.1/24",
    dhcpSettings:{
        enable:true,
        ipaddrStart:"10.10.97.100",
        ipaddrEnd:"10.10.97.250",
        leasetime:120,
        dhcpns:"auto"
    },
    interfaceIds:[$p2],
    igmpSnoopEnable:false
}')
echo "$POST_VLAN_L3" > "$OUT_DIR/post-p-vlan-l3-body.json"

echo "[P] Probe P: POST purpose=vlan + gatewaySubnet + dhcpSettings (v6 L3 vlan hypothesis)" >&2
"${CURL[@]}" -X POST "$(api "/sites/$SITE_ID/setting/lan/networks")" \
    -H 'Content-Type: application/json' \
    -d "$POST_VLAN_L3" \
    > "$OUT_DIR/post-p-vlan-l3-response.json"
jq '.' "$OUT_DIR/post-p-vlan-l3-response.json" >&2
NEW_P_ID=$(jq -r 'if .errorCode == 0 then .result else empty end' "$OUT_DIR/post-p-vlan-l3-response.json" 2>/dev/null || true)

# If P succeeded, read back the network detail to see if gateway/DHCP stuck.
if [[ -n "$NEW_P_ID" ]]; then
    echo "  [P] created id=$NEW_P_ID — reading back via list to verify L3 fields..." >&2
    "${CURL[@]}" "$(api "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")" \
        | jq --arg id "$NEW_P_ID" '.result.data[] | select(.id==$id) | {id, name, purpose, vlan, gatewaySubnet, dhcpSettings, interfaceIds, allLan, interface}' \
        > "$OUT_DIR/post-p-vlan-l3-readback.json"
    jq '.' "$OUT_DIR/post-p-vlan-l3-readback.json" >&2
fi

# Cleanup: delete all probe test networks.
for cid in "$NEW_NET_ID" "$NEW_VLAN_ID" "$NEW_M_ID" "$NEW_N_ID" "$NEW_O_ID" "$NEW_P_ID"; do
    [[ -z "$cid" ]] && continue
    echo "  [cleanup] DELETE $cid" >&2
    "${CURL[@]}" -X DELETE "$(api "/sites/$SITE_ID/setting/lan/networks/$cid")" | jq '{errorCode, msg}' >&2
done

echo "" >&2
echo "=== POST-PROBE SUMMARY ===" >&2
echo "Probe J (POST interface + interfaceIds):   $OUT_DIR/post-interface-response.json" >&2
echo "Probe K (POST vlan baseline):              $OUT_DIR/post-vlan-response.json" >&2
echo "Probe L (PATCH vlan->interface):           $OUT_DIR/patch-vlan-to-interface-response.json" >&2
echo "Probe M (POST interface, no interfaceIds): $OUT_DIR/post-m-noports-response.json" >&2
echo "Probe N (POST interface + deviceType=1):   $OUT_DIR/post-n-devtype-response.json" >&2
echo "Probe O (POST interface + interfaceIds=[]): $OUT_DIR/post-o-emptyids-response.json" >&2
echo "Probe P (POST vlan + L3 settings):         $OUT_DIR/post-p-vlan-l3-response.json" >&2
echo "" >&2

# Final state via list endpoint (individual GET returns -1600).
echo "" >&2
echo "[POST] cameras current state (via list endpoint):" >&2
"${CURL[@]}" "$(api "/sites/$SITE_ID/setting/lan/networks?currentPage=1&currentPageSize=100")" \
    | jq '.result.data[] | select(.id=="'"$CAMERAS_ID"'") | {id, name, purpose, vlan, gatewaySubnet, dhcpSettings, interfaceIds, allLan, subnetOverride, subnetOverrideEnable}'
