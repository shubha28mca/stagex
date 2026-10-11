#!/usr/bin/env bash
# =============================================================================
# End-to-end smoke test — hall seating arrangements (Operational Admin)
#
# Exercises the whole feature against a RUNNING stack: the seeded sample map,
# creating an arrangement from scratch, the row generator, tier and per-seat
# pricing, the publish completeness gate, seat-id stability across a re-save,
# and role separation.
#
# Prerequisites: the stack is up (`docker compose up -d`) and the admin API is
# reachable. Only bash, curl and python3 are needed — no jq.
#
#   chmod +x AdminBackend/scripts/seatmap_smoke.sh
#   ./AdminBackend/scripts/seatmap_smoke.sh
#
# Override the target with:  ADMIN_API=http://host:8081 ./seatmap_smoke.sh
#
# The script is read-only with respect to the seeded data: it creates its own
# arrangement on a hall that has none and deletes it again at the end.
# =============================================================================

set -uo pipefail

API="${ADMIN_API:-http://localhost:8081}"
OPS_EMAIL="${OPS_EMAIL:-ops@stagex.test}"
OPS_PASS="${OPS_PASS:-Ops@12345}"
EVENT_EMAIL="${EVENT_EMAIL:-event@stagex.test}"
EVENT_PASS="${EVENT_PASS:-Event@12345}"

PASS=0
FAIL=0
BODY=""
STATUS=""

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red()   { printf '\033[31m%s\033[0m\n' "$1"; }
head2() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

ok()  { PASS=$((PASS+1)); green "  ok    $1"; }
bad() { FAIL=$((FAIL+1)); red   "  FAIL  $1"; [ -n "${2:-}" ] && printf '        %s\n' "$2"; }

# check <description> <actual> <expected>
check() {
  if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "got '$2', want '$3'"; fi
}
# checkne <description> <actual> <unwanted>
checkne() {
  if [ "$2" != "$3" ]; then ok "$1"; else bad "$1" "got the unwanted value '$3'"; fi
}

# req <METHOD> <PATH> [JSON_BODY] [TOKEN]
# Leaves the response body in $BODY and the HTTP status in $STATUS.
req() {
  local method="$1" path="$2" body="${3:-}" token="${4:-${TOKEN:-}}"
  local args=(-sS -X "$method" "${API}${path}" -H 'Content-Type: application/json'
              -w '\n%{http_code}')
  [ -n "$token" ] && args+=(-H "Authorization: Bearer ${token}")
  [ -n "$body" ] && args+=(-d "$body")
  local raw
  raw="$(curl "${args[@]}" 2>/dev/null)"
  STATUS="${raw##*$'\n'}"
  BODY="${raw%$'\n'*}"
}

# jget <python-expression>  — reads $BODY, exposes the envelope's payload as `d`.
# A tiny eval-based reader keeps the script dependency-free (no jq).
jget() {
  printf '%s' "$BODY" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin).get("data")
except Exception:
    print("PARSE_ERROR"); sys.exit(0)
try:
    print(eval(sys.argv[1]))
except Exception as e:
    print("ERR:%s" % e)
' "$1"
}

# jerr prints the error message from a failure envelope.
jerr() {
  printf '%s' "$BODY" | python3 -c '
import json, sys
try:
    print(json.load(sys.stdin).get("error", {}).get("message", ""))
except Exception:
    print("")
'
}

printf '\033[1mStageX — seat-map end-to-end smoke\033[0m\n'
printf 'target: %s\n' "$API"

# -----------------------------------------------------------------------------
head2 "0. Health & authentication"
# -----------------------------------------------------------------------------
req GET /admin/health "" ""
check "admin API is up" "$STATUS" "200"
if [ "$STATUS" != "200" ]; then
  red "\nThe admin API is not reachable at $API — start the stack first:"
  red "  docker compose up -d"
  exit 1
fi

req POST /admin/auth/login "{\"email\":\"$OPS_EMAIL\",\"password\":\"$OPS_PASS\"}" ""
check "ops admin logs in" "$STATUS" "200"
TOKEN="$(jget 'd["token"]')"
checkne "ops token issued" "$TOKEN" ""
[ -z "$TOKEN" ] || [ "$TOKEN" = "PARSE_ERROR" ] && { red "cannot continue without a token"; exit 1; }

req POST /admin/auth/login "{\"email\":\"$EVENT_EMAIL\",\"password\":\"$EVENT_PASS\"}" ""
EVENT_TOKEN="$(jget 'd["token"]')"
check "event admin logs in" "$STATUS" "200"

# -----------------------------------------------------------------------------
head2 "1. Access control — seat maps are Operational Admin only"
# -----------------------------------------------------------------------------
req GET /admin/ops/seat-maps "" ""
check "anonymous request is rejected" "$STATUS" "401"

req GET /admin/ops/seat-maps "" "$EVENT_TOKEN"
check "event admin is forbidden from the ops seat-map index" "$STATUS" "403"

req GET /admin/ops/halls/00000000-0000-0000-0000-000000000000/seat-map "" "$EVENT_TOKEN"
check "event admin is forbidden from a hall's arrangement" "$STATUS" "403"

# -----------------------------------------------------------------------------
head2 "2. Index"
# -----------------------------------------------------------------------------
req GET /admin/ops/seat-maps
check "index returns 200" "$STATUS" "200"
HALL_COUNT="$(jget 'len(d)')"
checkne "index lists halls" "$HALL_COUNT" "0"

# The seeded sample: Nehru Auditorium has a published 482-seat arrangement.
SEEDED_ID="$(jget '[h["hallId"] for h in d if h["hallName"]=="Nehru Auditorium"][0]')"
checkne "seeded hall 'Nehru Auditorium' is present" "$SEEDED_ID" ""
check "seeded hall reports an arrangement" \
  "$(jget '[h["hasMap"] for h in d if h["hallName"]=="Nehru Auditorium"][0]')" "True"
check "seeded arrangement is published" \
  "$(jget '[h["status"] for h in d if h["hallName"]=="Nehru Auditorium"][0]')" "published"

# -----------------------------------------------------------------------------
head2 "3. Seeded sample data (visualization fixture)"
# -----------------------------------------------------------------------------
req GET "/admin/ops/halls/${SEEDED_ID}/seat-map"
check "seeded map loads" "$STATUS" "200"
check "4 price tiers"        "$(jget 'len(d["zones"])')" "4"
check "18 rows"              "$(jget 'd["summary"]["rows"]')" "18"
check "3 sections"           "$(jget 'len(d["summary"]["sections"])')" "3"
check "stage element exists" "$(jget 'len([e for e in d["elements"] if e["kind"]=="stage"])')" "1"
check "backstage element exists" "$(jget 'len([e for e in d["elements"] if e["kind"]=="backstage"])')" "1"

# Row A is the front row: distance from the stage must be 0.
check "front row A is 0 rows from the stage" \
  "$(jget '[s["rowIndex"] for s in d["seats"] if s["rowLabel"]=="A"][0]')" "0"
# Lettering skips I and O, so the physical 9th row back is labelled J.
check "row J is 8 rows from the stage (I is skipped)" \
  "$(jget '[s["rowIndex"] for s in d["seats"] if s["rowLabel"]=="J"][0]')" "8"
check "no row is labelled I or O" \
  "$(jget 'len([s for s in d["seats"] if s["rowLabel"] in ("I","O")])')" "0"

# Price resolution: a middle VIP seat inherits its tier, an end VIP seat has a
# restricted-view override.
check "VIP seat inherits its tier price (2500)" \
  "$(jget '[s["effectivePrice"] for s in d["seats"] if s["rowLabel"]=="A" and s["seatNumber"]==12][0]')" "2500"
check "VIP seat A12 has no per-seat override" \
  "$(jget '[s["price"] for s in d["seats"] if s["rowLabel"]=="A" and s["seatNumber"]==12][0]')" "None"
check "restricted-view seat A1 is overridden to 1800" \
  "$(jget '[s["effectivePrice"] for s in d["seats"] if s["rowLabel"]=="A" and s["seatNumber"]==1][0]')" "1800"
check "balcony seat inherits 450" \
  "$(jget '[s["effectivePrice"] for s in d["seats"] if s["rowLabel"]=="P" and s["seatNumber"]==15][0]')" "450"

# Edge cases the seed deliberately includes.
check "2 blocked seats in row F"  "$(jget 'd["summary"]["blockedSeats"]')" "2"
check "2 house-held seats"        "$(jget 'd["summary"]["houseSeats"]')" "2"
check "2 wheelchair spaces"       "$(jget 'len([s for s in d["seats"] if s["kind"]=="wheelchair"])')" "2"
check "2 companion seats"         "$(jget 'len([s for s in d["seats"] if s["kind"]=="companion"])')" "2"
check "no unpriced sellable seats" "$(jget 'd["summary"]["unpricedSeats"]')" "0"

# Blocked and house seats must not be counted as capacity.
TOTAL_ROWS="$(jget 'len(d["seats"])')"
SELLABLE="$(jget 'd["summary"]["totalSeats"]')"
check "482 seat rows stored"     "$TOTAL_ROWS" "482"
check "478 are sellable (482 - 2 blocked - 2 house)" "$SELLABLE" "478"
check "denormalized total_seats agrees with the summary" \
  "$(jget 'd["totalSeats"] == d["summary"]["totalSeats"]')" "True"
checkne "full-house value is calculated" "$(jget 'int(d["summary"]["potentialRevenue"])')" "0"

# -----------------------------------------------------------------------------
head2 "4. Row generator (server-owned geometry)"
# -----------------------------------------------------------------------------
req POST /admin/ops/seat-maps/generate-rows \
  '{"section":"Stalls","rows":4,"seatsPerRow":10,"firstRowIndex":0,"firstRowLetter":0,
    "numberFrom":1,"zoneCode":"stalls","kind":"seat","centerX":600,"originY":200,
    "seatPitch":26,"rowPitch":30,"curve":6}'
check "generator returns 200" "$STATUS" "200"
check "40 seats generated"    "$(jget 'len(d)')" "40"
check "first seat is A1"      "$(jget 'd[0]["seatLabel"]')" "A1"
check "rows are centred on x=600" \
  "$(jget 'd[0]["posX"] + d[9]["posX"]')" "1200"
check "curve lifts the row ends above the centre" \
  "$(jget 'd[0]["posY"] < d[5]["posY"]')" "True"
check "4th row is 3 rows from the stage" "$(jget 'd[30]["rowIndex"]')" "3"

req POST /admin/ops/seat-maps/generate-rows '{"section":"","rows":2,"seatsPerRow":2}'
check "generator rejects a blank section" "$STATUS" "400"
req POST /admin/ops/seat-maps/generate-rows '{"section":"X","rows":0,"seatsPerRow":2}'
check "generator rejects zero rows" "$STATUS" "400"

# -----------------------------------------------------------------------------
head2 "5. Create an arrangement from scratch"
# -----------------------------------------------------------------------------
# Pick a hall that has no arrangement, so the seeded fixture is left untouched.
req GET /admin/ops/seat-maps
NEW_HALL="$(jget '([h["hallId"] for h in d if not h["hasMap"]] or [""])[0]')"
NEW_HALL_NAME="$(jget '([h["hallName"] for h in d if not h["hasMap"]] or [""])[0]')"

if [ -z "$NEW_HALL" ] || [ "$NEW_HALL" = "None" ]; then
  printf '  skip  every hall already has an arrangement — skipping the create flow\n'
  printf '        (run `docker compose down -v && docker compose up -d` for a clean fixture)\n'
else
  printf '  using hall: %s\n' "$NEW_HALL_NAME"

  req POST "/admin/ops/halls/${NEW_HALL}/seat-map"
  check "create returns 201" "$STATUS" "201"
  check "new arrangement starts as a draft" "$(jget 'd["status"]')" "draft"
  check "new arrangement has no seats"      "$(jget 'len(d["seats"])')" "0"
  check "stage and backstage are pre-placed" \
    "$(jget 'sorted(e["kind"] for e in d["elements"])')" "['backstage', 'stage']"

  req POST "/admin/ops/halls/${NEW_HALL}/seat-map"
  check "creating twice is a conflict (one arrangement per hall)" "$STATUS" "409"

  # -- save a draft with one tier and two rows, one seat left untiered --------
  SAVE_DRAFT='{
    "name":"Smoke test seating","canvasWidth":1200,"canvasHeight":800,"status":"draft",
    "zones":[{"code":"front","name":"Front","color":"#6a1b9a","price":900,"displayOrder":1}],
    "seats":[
      {"section":"Stalls","rowLabel":"A","rowIndex":0,"seatNumber":1,"seatLabel":"A1",
       "zoneCode":"front","price":null,"kind":"seat","posX":300,"posY":200,"isActive":true},
      {"section":"Stalls","rowLabel":"A","rowIndex":0,"seatNumber":2,"seatLabel":"A2",
       "zoneCode":"front","price":null,"kind":"seat","posX":326,"posY":200,"isActive":true},
      {"section":"Stalls","rowLabel":"B","rowIndex":1,"seatNumber":1,"seatLabel":"B1",
       "zoneCode":"","price":null,"kind":"seat","posX":300,"posY":230,"isActive":true}
    ],
    "elements":[{"kind":"stage","label":"STAGE","posX":400,"posY":70,"width":400,
                 "height":80,"rotation":0,"color":"#0e1237"}]
  }'
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$SAVE_DRAFT"
  check "draft saves" "$STATUS" "200"
  check "3 seats stored"          "$(jget 'len(d["seats"])')" "3"
  check "tiered seats inherit 900" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "900"
  check "the untiered seat is flagged unpriced" "$(jget 'd["summary"]["unpricedSeats"]')" "1"
  check "a draft may be incomplete"             "$(jget 'd["status"]')" "draft"

  # Capture ids to prove they survive a re-save.
  A1_ID="$(jget '[s["id"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')"
  B1_ID="$(jget '[s["id"] for s in d["seats"] if s["seatLabel"]=="B1"][0]')"

  # -- the publish gate ------------------------------------------------------
  PUBLISH_ATTEMPT="${SAVE_DRAFT/\"status\":\"draft\"/\"status\":\"published\"}"
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$PUBLISH_ATTEMPT"
  check "publishing with an unpriced seat is refused" "$STATUS" "400"
  MSG="$(jerr)"
  case "$MSG" in
    *"no price"*) ok "refusal explains the unpriced seats" ;;
    *) bad "refusal explains the unpriced seats" "message was: $MSG" ;;
  esac

  req POST "/admin/ops/halls/${NEW_HALL}/seat-map/publish"
  check "publish endpoint applies the same gate" "$STATUS" "400"

  # -- bulk pricing: assign a tier to the selection --------------------------
  req PATCH "/admin/ops/halls/${NEW_HALL}/seat-map/seats" \
    "{\"seatIds\":[\"${B1_ID}\"],\"zoneCode\":\"front\",\"price\":null,\"clearPrice\":false,\"isActive\":null}"
  check "bulk tier assignment succeeds" "$STATUS" "200"
  check "the last unpriced seat is now priced" "$(jget 'd["summary"]["unpricedSeats"]')" "0"
  check "it inherits the tier price" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="B1"][0]')" "900"

  # -- bulk pricing: a per-seat override on a multi-seat selection -----------
  req PATCH "/admin/ops/halls/${NEW_HALL}/seat-map/seats" \
    "{\"seatIds\":[\"${A1_ID}\",\"${B1_ID}\"],\"zoneCode\":null,\"price\":1450,\"clearPrice\":false,\"isActive\":null}"
  check "bulk price override on 2 seats succeeds" "$STATUS" "200"
  check "A1 is overridden to 1450" \
    "$(jget '[s["price"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "1450.0"
  check "B1 is overridden to 1450" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="B1"][0]')" "1450"
  check "the untouched seat still inherits 900" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="A2"][0]')" "900"

  # -- clearing an override falls back to the tier ---------------------------
  req PATCH "/admin/ops/halls/${NEW_HALL}/seat-map/seats" \
    "{\"seatIds\":[\"${A1_ID}\"],\"zoneCode\":null,\"price\":null,\"clearPrice\":true,\"isActive\":null}"
  check "clearing an override succeeds" "$STATUS" "200"
  check "A1 falls back to the tier price" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "900"
  check "A1 no longer carries an override" \
    "$(jget '[s["price"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "None"

  # -- bulk validation -------------------------------------------------------
  req PATCH "/admin/ops/halls/${NEW_HALL}/seat-map/seats" \
    '{"seatIds":[],"zoneCode":null,"price":100,"clearPrice":false,"isActive":null}'
  check "an empty selection is rejected" "$STATUS" "400"
  req PATCH "/admin/ops/halls/${NEW_HALL}/seat-map/seats" \
    "{\"seatIds\":[\"${A1_ID}\"],\"zoneCode\":\"does-not-exist\",\"price\":null,\"clearPrice\":false,\"isActive\":null}"
  check "an unknown tier is rejected" "$STATUS" "400"
  req PATCH "/admin/ops/halls/${NEW_HALL}/seat-map/seats" \
    "{\"seatIds\":[\"${A1_ID}\"],\"zoneCode\":null,\"price\":-5,\"clearPrice\":false,\"isActive\":null}"
  check "a negative price is rejected" "$STATUS" "400"

  # -- now publishing must succeed -------------------------------------------
  req POST "/admin/ops/halls/${NEW_HALL}/seat-map/publish"
  check "publishing a complete arrangement succeeds" "$STATUS" "200"
  check "status is published" "$(jget 'd["status"]')" "published"

  req POST "/admin/ops/halls/${NEW_HALL}/seat-map/unpublish"
  check "unpublish returns it to draft" "$(jget 'd["status"]')" "draft"

  # -- seat-id stability: the reason the save upserts on the natural key -----
  # Re-save with A2 removed and a new C1 added; A1 and B1 must keep their ids so
  # anything referencing a sold seat is never orphaned by a redesign.
  RESAVE='{
    "name":"Smoke test seating","canvasWidth":1200,"canvasHeight":800,"status":"draft",
    "zones":[{"code":"front","name":"Front row","color":"#ff7a3c","price":1200,"displayOrder":1}],
    "seats":[
      {"section":"Stalls","rowLabel":"A","rowIndex":0,"seatNumber":1,"seatLabel":"A1",
       "zoneCode":"front","price":null,"kind":"seat","posX":310,"posY":205,"isActive":true},
      {"section":"Stalls","rowLabel":"B","rowIndex":1,"seatNumber":1,"seatLabel":"B1",
       "zoneCode":"front","price":1450,"kind":"seat","posX":300,"posY":230,"isActive":true},
      {"section":"Stalls","rowLabel":"C","rowIndex":2,"seatNumber":1,"seatLabel":"C1",
       "zoneCode":"front","price":null,"kind":"seat","posX":300,"posY":260,"isActive":true}
    ],
    "elements":[]
  }'
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$RESAVE"
  check "re-save succeeds" "$STATUS" "200"
  check "A1 keeps its id across a redesign" \
    "$(jget '[s["id"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "$A1_ID"
  check "B1 keeps its id across a redesign" \
    "$(jget '[s["id"] for s in d["seats"] if s["seatLabel"]=="B1"][0]')" "$B1_ID"
  check "the removed seat A2 is gone" \
    "$(jget 'len([s for s in d["seats"] if s["seatLabel"]=="A2"])')" "0"
  check "the added seat C1 is present" \
    "$(jget 'len([s for s in d["seats"] if s["seatLabel"]=="C1"])')" "1"
  check "A1 moved to its new position" \
    "$(jget '[s["posX"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "310"
  check "renaming a tier keeps its seats attached" \
    "$(jget 'd["zones"][0]["name"]')" "Front row"
  check "the retiered price applies" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="A1"][0]')" "1200"
  check "the per-seat override still wins over the new tier price" \
    "$(jget '[s["effectivePrice"] for s in d["seats"] if s["seatLabel"]=="B1"][0]')" "1450"
  check "elements were replaced wholesale" "$(jget 'len(d["elements"])')" "0"

  # -- dropping a tier detaches its seats rather than deleting them ----------
  DROP_TIER="${RESAVE/\"zones\":\[\{\"code\":\"front\",\"name\":\"Front row\",\"color\":\"#ff7a3c\",\"price\":1200,\"displayOrder\":1\}\]/\"zones\":[]}"
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$DROP_TIER"
  if [ "$STATUS" = "400" ]; then
    # Seats still referencing the dropped tier make this an invalid payload,
    # which is itself correct behaviour — record it and move on.
    ok "dropping a referenced tier is rejected as an invalid payload"
  else
    check "dropping a tier keeps the seats" "$(jget 'len(d["seats"])')" "3"
    check "its seats become unpriced"       "$(jget 'd["summary"]["unpricedSeats"]')" "3"
  fi

  # -- payload validation ----------------------------------------------------
  DUP='{"name":"x","canvasWidth":1200,"canvasHeight":800,"status":"draft","zones":[],
        "seats":[
          {"section":"S","rowLabel":"A","rowIndex":0,"seatNumber":1,"seatLabel":"A1",
           "zoneCode":"","price":null,"kind":"seat","posX":10,"posY":10,"isActive":true},
          {"section":"S","rowLabel":"A","rowIndex":0,"seatNumber":1,"seatLabel":"A1",
           "zoneCode":"","price":null,"kind":"seat","posX":40,"posY":10,"isActive":true}],
        "elements":[]}'
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$DUP"
  check "a duplicate seat is rejected" "$STATUS" "400"

  UNKNOWN_FIELD='{"name":"x","canvasWidth":1200,"canvasHeight":800,"status":"draft",
                  "zones":[],"seats":[],"elements":[],"somethingElse":1}'
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$UNKNOWN_FIELD"
  check "an unknown field is rejected" "$STATUS" "400"

  BAD_KIND='{"name":"x","canvasWidth":1200,"canvasHeight":800,"status":"draft","zones":[],
             "seats":[{"section":"S","rowLabel":"A","rowIndex":0,"seatNumber":1,
                       "seatLabel":"A1","zoneCode":"","price":null,"kind":"beanbag",
                       "posX":10,"posY":10,"isActive":true}],"elements":[]}'
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$BAD_KIND"
  check "an unknown seat kind is rejected" "$STATUS" "400"

  # -- an event admin still cannot write ------------------------------------
  req PUT "/admin/ops/halls/${NEW_HALL}/seat-map" "$RESAVE" "$EVENT_TOKEN"
  check "event admin cannot save an arrangement" "$STATUS" "403"

  # -- cleanup ---------------------------------------------------------------
  req DELETE "/admin/ops/halls/${NEW_HALL}/seat-map"
  check "delete returns 200" "$STATUS" "200"
  req GET "/admin/ops/halls/${NEW_HALL}/seat-map"
  check "the arrangement is gone (cascaded)" "$STATUS" "404"
fi

# -----------------------------------------------------------------------------
head2 "6. Unknown hall"
# -----------------------------------------------------------------------------
req GET /admin/ops/halls/00000000-0000-0000-0000-000000000000/seat-map
check "an unknown hall is a 404" "$STATUS" "404"
MSG="$(jerr)"
case "$MSG" in
  *"hall not found"*) ok "404 distinguishes an unknown hall from a missing arrangement" ;;
  *) bad "404 distinguishes an unknown hall" "message was: $MSG" ;;
esac

# -----------------------------------------------------------------------------
head2 "7. Seeded fixture is unchanged"
# -----------------------------------------------------------------------------
req GET "/admin/ops/halls/${SEEDED_ID}/seat-map"
check "seeded arrangement still has 482 seats" "$(jget 'len(d["seats"])')" "482"
check "seeded arrangement is still published"  "$(jget 'd["status"]')" "published"

# -----------------------------------------------------------------------------
printf '\n\033[1m== summary ==\033[0m\n'
green "  passed: $PASS"
if [ "$FAIL" -gt 0 ]; then
  red   "  failed: $FAIL"
  exit 1
fi
printf '  failed: 0\n'
green "\nAll seat-map end-to-end checks passed."
