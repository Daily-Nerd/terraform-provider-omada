#!/usr/bin/env bash
# Compatible with bash 3.2 (macOS default) — no associative arrays.
# Issue #10 probe: figure out how Omada assigns ACL rule indices and whether
# Terraform can drive ordering deterministically.
#
# This script:
#   1. Captures the existing ACL rule list (snapshot for restore)
#   2. Creates 3 disabled probe rules with names PROBE_A / PROBE_B / PROBE_C
#      in that exact create order
#   3. Reads back what indices the controller assigned
#   4. Attempts to PATCH PROBE_C to move it BEFORE PROBE_A
#   5. Re-reads to see if the patch took effect
#   6. Tries an explicit reorder/sort endpoint if PATCH index didn't stick
#   7. Cleans up — deletes the 3 probe rules
#
# Usage:
#   export OMADA_URL='https://192.168.68.136'
#   export OMADA_USERNAME='Kibukx'
#   export OMADA_PASSWORD='...'
#   export OMADA_SITE='Yggdrasil'
#   ./scripts/probe-issue-10-acl-ordering.sh
#
# Output: dist/issue-10/*.json plus a markdown summary at the end.

set -euo pipefail

: "${OMADA_URL:?missing OMADA_URL}"
: "${OMADA_USERNAME:?missing OMADA_USERNAME}"
: "${OMADA_PASSWORD:?missing OMADA_PASSWORD}"
: "${OMADA_SITE:?missing OMADA_SITE}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/dist/issue-10"
mkdir -p "$OUT_DIR"

JAR=$(mktemp -t omada-issue10.XXXXXX)
trap 'rm -f "$JAR"' EXIT

CURL=(curl -sk -H "Accept: application/json")

# ---- Login ----
echo "[1/8] Login" >&2
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

ACL_BASE="/sites/$SITE_ID/setting/firewall/acls"

list_rules() {
    "${CURL[@]}" "$(api "$ACL_BASE?type=0&currentPage=1&currentPageSize=200")"
}

# ---- 2. Snapshot existing rules ----
echo "[2/8] Snapshot existing ACL list" >&2
list_rules > "$OUT_DIR/00-before.json"
EXISTING_COUNT=$(jq -r '.result.data | length' "$OUT_DIR/00-before.json")
echo "  existing rule count: $EXISTING_COUNT" >&2

# ---- 3. Provision two distinct IP groups for ACL src/dst.
# Why IP groups, not networks: vlan-purpose networks (L2-only, no gateway
# adopted) are not visible to the firewall engine yet. -33603 fires when
# the controller can't resolve the referenced network. IP groups are
# address objects independent of gateway adoption — work pre-ER707.
IPG_BASE="/sites/$SITE_ID/setting/firewall/ipGroups"

ensure_ip_group() {
    local name="$1" ip="$2"
    # Check if already exists
    local existing
    existing=$("${CURL[@]}" "$(api "$IPG_BASE?currentPage=1&currentPageSize=100")" \
        | jq -r --arg n "$name" '.result.data[] | select(.name==$n) | .id' | head -1)
    if [[ -n "$existing" && "$existing" != "null" ]]; then
        echo "$existing"
        return
    fi
    # Create new
    local body resp id
    body=$(jq -nc --arg name "$name" --arg ip "$ip" '{
        name: $name,
        type: 1,
        ipList: [{ ip: $ip, mask: 32, ipMaskType: 1, description: "issue-10 probe" }]
    }')
    resp=$("${CURL[@]}" -X POST "$(api "$IPG_BASE")" \
        -H 'Content-Type: application/json' -d "$body")
    id=$(echo "$resp" | jq -r '.result.id // .result // empty')
    if [[ -z "$id" || "$id" == "null" ]]; then
        echo "FAIL creating IP group $name: $(echo "$resp" | jq -r '.msg')" >&2
        echo "$resp" | jq '.' >&2
        exit 1
    fi
    echo "$id"
}

echo "  ensuring probe IP groups" >&2
IPG_SRC=$(ensure_ip_group "PROBE_SRC_GRP" "10.250.250.10")
IPG_DST=$(ensure_ip_group "PROBE_DST_GRP" "10.250.250.20")
echo "  src ip group: PROBE_SRC_GRP ($IPG_SRC)" >&2
echo "  dst ip group: PROBE_DST_GRP ($IPG_DST)" >&2

# ---- 4. Create 3 probe rules in order: A, B, C ----
make_rule() {
    local name="$1"
    # protocols: 1=ICMP, 6=TCP, 17=UDP (-1 rejected as -33608).
    # sourceType/destinationType: 0=network, 2=ip-group, 3=any.
    # Using IP groups (type=2) so the rule is valid pre-gateway adoption.
    jq -nc --arg name "$name" --arg src "$IPG_SRC" --arg dst "$IPG_DST" '{
        name: $name,
        type: 0,
        status: false,
        policy: 1,
        protocols: [6],
        sourceType: 2,
        sourceIds: [$src],
        destinationType: 2,
        destinationIds: [$dst],
        direction: { lanToWan: false, lanToLan: true },
        ipVersion: 4
    }'
}

echo "[3/8] Creating 3 probe rules: PROBE_A, PROBE_B, PROBE_C" >&2
PROBE_A_ID=""
PROBE_B_ID=""
PROBE_C_ID=""
for name in PROBE_A PROBE_B PROBE_C; do
    body=$(make_rule "$name")
    resp=$("${CURL[@]}" -X POST "$(api "$ACL_BASE")" \
        -H 'Content-Type: application/json' -d "$body")
    code=$(echo "$resp" | jq -r '.errorCode')
    if [[ "$code" != "0" ]]; then
        echo "  FAIL creating $name: $(echo "$resp" | jq -r '.msg')" >&2
        echo "$resp" | jq '.' >&2
        exit 1
    fi
    rule_id=$(echo "$resp" | jq -r '.result.id // .result.data.id // .result // empty')
    case "$name" in
        PROBE_A) PROBE_A_ID="$rule_id" ;;
        PROBE_B) PROBE_B_ID="$rule_id" ;;
        PROBE_C) PROBE_C_ID="$rule_id" ;;
    esac
    echo "  $name -> id=$rule_id" >&2
done

# ---- 5. Read back to observe assigned indices ----
PROBE_IDS_PATTERN="$PROBE_A_ID|$PROBE_B_ID|$PROBE_C_ID"
echo "[4/8] Reading post-create indices" >&2
list_rules > "$OUT_DIR/01-after-create.json"
jq --arg ids "$PROBE_IDS_PATTERN" '
    .result.data[]
    | select(.id | inside($ids))
    | {name, id, index}
' "$OUT_DIR/01-after-create.json" > "$OUT_DIR/01-probe-indices.json"
echo "  indices after create:" >&2
cat "$OUT_DIR/01-probe-indices.json" | sed 's/^/    /' >&2

# ---- 6. Try PATCH to move PROBE_C to lowest index ----
TARGET_ID="$PROBE_C_ID"
LOWEST=$(jq -r '[.result.data[].index] | min // 0' "$OUT_DIR/01-after-create.json")
echo "[5/8] PATCH PROBE_C ($TARGET_ID) to index=$LOWEST" >&2
patch_body=$(jq -nc --argjson idx "$LOWEST" '{index: $idx}')
"${CURL[@]}" -X PATCH "$(api "$ACL_BASE/$TARGET_ID")" \
    -H 'Content-Type: application/json' -d "$patch_body" \
    > "$OUT_DIR/02-patch-index.json"
patch_code=$(jq -r '.errorCode' "$OUT_DIR/02-patch-index.json")
patch_msg=$(jq -r '.msg' "$OUT_DIR/02-patch-index.json")
echo "  PATCH errorCode=$patch_code msg=\"$patch_msg\"" >&2

list_rules > "$OUT_DIR/03-after-patch.json"
echo "  indices after PATCH:" >&2
jq --arg ids "$PROBE_IDS_PATTERN" '
    .result.data[]
    | select(.id | inside($ids))
    | {name, id, index}
' "$OUT_DIR/03-after-patch.json" | sed 's/^/    /' >&2

# ---- 7. Try alternate reorder endpoints ----
echo "[6/8] Probing dedicated reorder endpoints" >&2

reorder_attempt() {
    local label="$1" method="$2" path="$3" body="$4"
    local out="$OUT_DIR/04-$label.json"
    local resp
    resp=$("${CURL[@]}" -X "$method" "$(api "$path")" \
        ${body:+-H "Content-Type: application/json"} ${body:+-d "$body"})
    echo "$resp" > "$out"
    local code=$(echo "$resp" | jq -r '.errorCode // "?"')
    echo "  $label: $method $path -> errorCode=$code" >&2
}

reorder_payload=$(jq -nc \
    --arg c "$PROBE_C_ID" \
    --arg a "$PROBE_A_ID" \
    --arg b "$PROBE_B_ID" \
    '{order: [$c, $a, $b]}')

reorder_attempt "post-order"   POST  "$ACL_BASE/order"          "$reorder_payload"
reorder_attempt "patch-order"  PATCH "$ACL_BASE/order"          "$reorder_payload"
reorder_attempt "post-sort"    POST  "$ACL_BASE/sort"           "$reorder_payload"
reorder_attempt "patch-batch"  PATCH "$ACL_BASE"                "$reorder_payload"

# Some Omada APIs use a /move endpoint with from/to indices
move_payload=$(jq -nc --argjson from 2 --argjson to 0 '{from: $from, to: $to}')
reorder_attempt "post-move"    POST  "$ACL_BASE/$PROBE_C_ID/move" "$move_payload"

# Final read
list_rules > "$OUT_DIR/05-after-reorder.json"

# ---- 8. Cleanup ----
echo "[7/8] Deleting probe rules + IP groups" >&2
for pair in "PROBE_A:$PROBE_A_ID" "PROBE_B:$PROBE_B_ID" "PROBE_C:$PROBE_C_ID"; do
    name="${pair%%:*}"
    rid="${pair##*:}"
    [[ -z "$rid" ]] && continue
    "${CURL[@]}" -X DELETE "$(api "$ACL_BASE/$rid")" > /dev/null
    echo "  deleted ACL $name ($rid)" >&2
done
for pair in "PROBE_SRC_GRP:$IPG_SRC" "PROBE_DST_GRP:$IPG_DST"; do
    name="${pair%%:*}"
    gid="${pair##*:}"
    [[ -z "$gid" ]] && continue
    "${CURL[@]}" -X DELETE "$(api "$IPG_BASE/$gid")" > /dev/null
    echo "  deleted IP group $name ($gid)" >&2
done

# ---- 9. Summary ----
echo "[8/8] Summary" >&2
{
    echo "# Issue #10 — ACL ordering investigation"
    echo
    echo "Generated: $(date -u +%FT%TZ)"
    echo
    echo "## Phase 1: indices assigned on creation"
    echo
    echo '```json'
    cat "$OUT_DIR/01-probe-indices.json"
    echo '```'
    echo
    echo "## Phase 2: PATCH index"
    echo
    echo "- target: PROBE_C ($TARGET_ID)"
    echo "- requested index: $LOWEST"
    echo "- response: errorCode=$patch_code msg=\"$patch_msg\""
    echo
    echo "Indices after PATCH:"
    echo
    echo '```json'
    jq --arg ids "$PROBE_IDS_PATTERN" '
        .result.data[]
        | select(.id | inside($ids))
        | {name, id, index}
    ' "$OUT_DIR/03-after-patch.json"
    echo '```'
    echo
    echo "## Phase 3: dedicated reorder endpoint probes"
    echo
    for f in "$OUT_DIR"/04-*.json; do
        label=$(basename "$f" .json | sed 's/^04-//')
        code=$(jq -r '.errorCode // "?"' "$f")
        msg=$(jq -r '.msg // ""' "$f")
        echo "- **$label**: errorCode=$code msg=\"$msg\""
    done
    echo
    echo "## Verdict heuristic"
    echo
    echo "Compare phase 1 indices against creation order (A, B, C). If indices ascend strictly with creation order, the API respects declaration order on insert."
    echo
    echo "If the PATCH in phase 2 returned errorCode=0 AND the post-PATCH index of PROBE_C matches the requested value, terraform CAN drive ordering via PATCH."
    echo
    echo "If neither — outcome C in issue #10 (controller-managed ordering, document the limitation)."
} > "$OUT_DIR/SUMMARY.md"

echo
echo "Done. Summary: $OUT_DIR/SUMMARY.md" >&2
echo "Raw payloads: $OUT_DIR/*.json" >&2
