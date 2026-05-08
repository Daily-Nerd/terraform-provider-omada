#!/usr/bin/env bash
# Audits which fields the controller treats as required for omada_network
# creation. Uses purpose=vlan as the baseline (works against empty
# controllers). For each candidate "Optional" field, runs a CREATE request
# without that field and records whether the controller accepts (-> truly
# optional) or rejects with -1001 (-> required but masquerading as Optional).
#
# Output: dist/audit/network-schema-report.json
#
# Usage: ./scripts/audit-network-schema.sh

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/audit"
JAR=$(mktemp -t omada-audit.XXXXXX)
trap 'rm -f "$JAR"' EXIT
mkdir -p "$OUT_DIR"

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

# Site
SITES=$("${CURL[@]}" -b "$JAR" "$OMADA_URL/$OMADAC_ID/api/v2/sites?token=$TOKEN&currentPage=1&currentPageSize=100")
SITE_ID=$(echo "$SITES" | jq -r --arg name "$OMADA_SITE" '.result.data[] | select(.name == $name) | .id' | head -1)
echo "siteId=$SITE_ID" >&2
echo

CREATE_URL="$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/setting/lan/networks?token=$TOKEN"

# Build the maximal "kitchen sink" payload that we KNOW works
# (vlan-purpose with all known fields filled in).
build_full() {
    jq -nc '{
        name: $name,
        purpose: "vlan",
        vlan: ($vlan | tonumber),
        isolation: false,
        igmpSnoopEnable: false,
        fastLeaveEnable: false,
        mldSnoopEnable: false,
        dhcpv6Guard: {enable: false},
        dhcpL2RelayEnable: false,
        dhcpGuard: {enable: false},
        portal: false,
        accessControlRule: false,
        rateLimit: false,
        arpDetectionEnable: false
    }' --arg name "$1" --arg vlan "$2"
}

cleanup_test() {
    local name="$1"
    local list nid
    list=$("${CURL[@]}" -b "$JAR" "$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/setting/lan/networks?token=$TOKEN&currentPage=1&currentPageSize=100")
    nid=$(echo "$list" | jq -r --arg n "$name" '.result.data[] | select(.name == $n) | .id' | head -1)
    if [[ -n "$nid" && "$nid" != "null" ]]; then
        "${CURL[@]}" -b "$JAR" -X DELETE "$OMADA_URL/$OMADAC_ID/api/v2/sites/$SITE_ID/setting/lan/networks/$nid?token=$TOKEN" >/dev/null
    fi
}

# Probe each field by omitting it from the payload
probe() {
    local field="$1"
    local name="audit-$field"
    local body
    body=$(build_full "$name" 980 | jq "del(.$field)")
    local resp
    resp=$("${CURL[@]}" -b "$JAR" -X POST "$CREATE_URL" \
        -H 'Content-Type: application/json' -d "$body")
    local code msg
    code=$(echo "$resp" | jq -r '.errorCode')
    msg=$(echo "$resp" | jq -r '.msg // ""')
    cleanup_test "$name"
    printf '%s\t%s\t%s\n' "$field" "$code" "$msg"
}

echo "Probing each field via field-omission test (purpose=vlan baseline)..." >&2
echo

# Header
printf "FIELD\tERRORCODE\tMSG\n" > "$OUT_DIR/network-schema-report.tsv"

# Baseline (full payload — should succeed)
echo "=== baseline (all fields present) ===" >&2
BODY=$(build_full "audit-baseline" 989)
RESP=$("${CURL[@]}" -b "$JAR" -X POST "$CREATE_URL" -H 'Content-Type: application/json' -d "$BODY")
echo "$RESP" | jq '{errorCode, msg}'
cleanup_test "audit-baseline"
echo

# Probe each known field
echo "=== probing each field (omit one at a time) ===" >&2
for field in isolation igmpSnoopEnable fastLeaveEnable mldSnoopEnable dhcpv6Guard dhcpL2RelayEnable dhcpGuard portal accessControlRule rateLimit arpDetectionEnable; do
    line=$(probe "$field")
    echo "$line" >&2
    echo "$line" >> "$OUT_DIR/network-schema-report.tsv"
done

echo
echo "Report: $OUT_DIR/network-schema-report.tsv" >&2
column -t -s $'\t' "$OUT_DIR/network-schema-report.tsv"

# Generate JSON report alongside
{
    echo '{'
    echo '  "controller_url": "'"$OMADA_URL"'",'
    echo '  "controller_ver": "'"$(curl -sk "$OMADA_URL/api/info" | jq -r '.result.controllerVer')"'",'
    echo '  "site": "'"$OMADA_SITE"'",'
    echo '  "site_id": "'"$SITE_ID"'",'
    echo '  "generated_at": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'",'
    echo '  "fields": ['
    first=1
    while IFS=$'\t' read -r field code msg; do
        [[ "$field" == "FIELD" ]] && continue
        if (( first == 0 )); then echo ","; fi
        first=0
        printf '    {"field": "%s", "errorCode": %s, "required_by_api": %s, "msg": "%s"}' \
            "$field" "$code" \
            "$([ "$code" != "0" ] && echo true || echo false)" \
            "$msg"
    done < "$OUT_DIR/network-schema-report.tsv"
    echo
    echo '  ]'
    echo '}'
} > "$OUT_DIR/network-schema-report.json"

echo
echo "JSON report: $OUT_DIR/network-schema-report.json" >&2
