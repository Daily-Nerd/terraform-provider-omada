#!/usr/bin/env bash
# Probe whether the Omada controller validates interfaceIds strictly,
# or stores any string. Tells us if we can dev-test without an adopted
# gateway by passing fake IDs.
#
# Usage:
#   export OMADA_URL=...
#   export OMADA_USERNAME=...
#   export OMADA_PASSWORD=...
#   export OMADA_SITE='Home'
#   ./scripts/test-network-create.sh
#
# Cleans up the test network after each attempt.

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

JAR=$(mktemp -t omada-test.XXXXXX)
trap 'rm -f "$JAR"' EXIT

CURL=(curl -sk -H "Accept: application/json")

# omadacId
OMADAC_ID=$(curl -sk "$OMADA_URL/api/info" | jq -r '.result.omadacId')
echo "omadacId=$OMADAC_ID" >&2

# login
LOGIN=$("${CURL[@]}" -c "$JAR" -X POST "$OMADA_URL/$OMADAC_ID/api/v2/login" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg u "$OMADA_USERNAME" --arg p "$OMADA_PASSWORD" '{username:$u, password:$p}')")
TOKEN=$(echo "$LOGIN" | jq -r '.result.token')
[[ -z "$TOKEN" || "$TOKEN" == "null" ]] && { echo "login failed"; exit 1; }
CURL+=(-H "Csrf-Token: $TOKEN")

# Resolve siteId
SITES=$("${CURL[@]}" -b "$JAR" "$OMADA_URL/$OMADAC_ID/api/v2/sites?token=$TOKEN&currentPage=1&currentPageSize=100")
SITE_ID=$(echo "$SITES" | jq -r --arg name "$OMADA_SITE" '.result.data[] | select(.name == $name) | .id' | head -1)
echo "siteId=$SITE_ID" >&2
echo

create_url="$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/setting/lan/networks?token=$TOKEN"

cleanup_test() {
    local name="$1"
    local list nid
    list=$("${CURL[@]}" -b "$JAR" "$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/setting/lan/networks?token=$TOKEN&currentPage=1&currentPageSize=100")
    nid=$(echo "$list" | jq -r --arg n "$name" '.result.data[] | select(.name == $n) | .id' | head -1)
    if [[ -n "$nid" && "$nid" != "null" ]]; then
        echo "  cleanup: deleting $name (id=$nid)" >&2
        "${CURL[@]}" -b "$JAR" -X DELETE "$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/setting/lan/networks/$nid?token=$TOKEN" | jq '{errorCode, msg}'
    fi
}

run_case() {
    local label="$1"
    local body="$2"
    local name
    name=$(echo "$body" | jq -r '.name')
    echo "=== $label ===" >&2
    echo "POST body:" >&2
    echo "$body" | jq '.' >&2
    echo "Response:" >&2
    "${CURL[@]}" -b "$JAR" -X POST "$create_url" \
        -H 'Content-Type: application/json' \
        -d "$body" \
        | jq '{errorCode, msg, result: (.result // null)}'
    echo
    cleanup_test "$name"
    echo
}

# A: no interfaceIds at all
run_case "A. interface-purpose, no interfaceIds field" "$(jq -nc '{
    name: "tf-test-a-no-iface",
    purpose: "interface",
    vlan: 991,
    gatewaySubnet: "10.99.91.1/24",
    igmpSnoopEnable: false,
    fastLeaveEnable: false,
    mldSnoopEnable: false,
    isolation: false,
    dhcpSettings: {enable: true, ipaddrStart: "10.99.91.100", ipaddrEnd: "10.99.91.200"}
}')"

# B: empty interfaceIds list
run_case "B. interface-purpose, interfaceIds: []" "$(jq -nc '{
    name: "tf-test-b-empty-iface",
    purpose: "interface",
    vlan: 992,
    gatewaySubnet: "10.99.92.1/24",
    interfaceIds: [],
    igmpSnoopEnable: false,
    fastLeaveEnable: false,
    mldSnoopEnable: false,
    isolation: false,
    dhcpSettings: {enable: true, ipaddrStart: "10.99.92.100", ipaddrEnd: "10.99.92.200"}
}')"

# C: fake string IDs
run_case "C. interface-purpose, interfaceIds: [\"fake-id-12345\"]" "$(jq -nc '{
    name: "tf-test-c-fake-iface",
    purpose: "interface",
    vlan: 993,
    gatewaySubnet: "10.99.93.1/24",
    interfaceIds: ["fake-id-12345"],
    igmpSnoopEnable: false,
    fastLeaveEnable: false,
    mldSnoopEnable: false,
    isolation: false,
    dhcpSettings: {enable: true, ipaddrStart: "10.99.93.100", ipaddrEnd: "10.99.93.200"}
}')"

# D: vlan-purpose control (should succeed regardless of gateway)
run_case "D. vlan-purpose (control case, should succeed)" "$(jq -nc '{
    name: "tf-test-d-vlan-only",
    purpose: "vlan",
    vlan: 994,
    igmpSnoopEnable: false,
    fastLeaveEnable: false,
    mldSnoopEnable: false,
    isolation: false
}')"
