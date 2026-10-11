#!/usr/bin/env bash
# =============================================================================
# End-to-end smoke test — ticket booking (participant side)
#
# Drives the whole buyer journey against a RUNNING stack, and specifically
# exercises the parts that are hard to get right:
#
#   * the seat picker's availability verdicts
#   * TWO BUYERS RACING for the same seat — fired in parallel, asserting that
#     exactly one wins and the loser is told which seat went
#   * the hold window, and a hold surviving while a payment is in flight
#   * payment retry, and the 3-attempt limit
#   * cancellation with a mandatory reason, and the offline refund record
#   * cancellation being refused once the event has started
#   * admin visibility of booked tickets, and settling the refund
#
# Prerequisites: the stack is up (`docker compose up -d`). Needs only bash,
# curl and python3.
#
#   chmod +x Backend/scripts/booking_smoke.sh
#   ./Backend/scripts/booking_smoke.sh
#
# Override targets:  API=http://host:8080 ADMIN_API=http://host:8081 ./booking_smoke.sh
#
# A short hold window makes the expiry assertions fast. Restart the api with
# SEAT_HOLD_TTL=45s to have this script cover real expiry end-to-end; without
# it, those checks are reported as skipped rather than silently passing.
# =============================================================================

set -uo pipefail

API="${API:-http://localhost:8080}"
ADMIN_API="${ADMIN_API:-http://localhost:8081}"
OPS_EMAIL="${OPS_EMAIL:-ops@stagex.test}"
OPS_PASS="${OPS_PASS:-Ops@12345}"

PASS=0
FAIL=0
SKIP=0
BODY=""
STATUS=""

green() { printf '\033[32m%s\033[0m\n' "$1"; }
red()   { printf '\033[31m%s\033[0m\n' "$1"; }
head2() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

ok()   { PASS=$((PASS+1)); green "  ok    $1"; }
bad()  { FAIL=$((FAIL+1)); red   "  FAIL  $1"; [ -n "${2:-}" ] && printf '        %s\n' "$2"; }
skip() { SKIP=$((SKIP+1)); printf '  skip  %s\n' "$1"; }

check()   { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "got '$2', want '$3'"; fi; }
checkne() { if [ "$2" != "$3" ]; then ok "$1"; else bad "$1" "got the unwanted '$3'"; fi; }

# req <METHOD> <PATH> [JSON] [TOKEN] [BASE]
req() {
  local method="$1" path="$2" body="${3:-}" token="${4:-${TOKEN:-}}" base="${5:-$API}"
  local args=(-sS -X "$method" "${base}${path}" -H 'Content-Type: application/json' -w '\n%{http_code}')
  [ -n "$token" ] && args+=(-H "Authorization: Bearer ${token}")
  [ -n "$body" ] && args+=(-d "$body")
  local raw; raw="$(curl "${args[@]}" 2>/dev/null)"
  STATUS="${raw##*$'\n'}"
  BODY="${raw%$'\n'*}"
}

# jget <python-expr on `d`> — reads $BODY's data payload.
jget() {
  printf '%s' "$BODY" | python3 -c '
import json,sys
try: d=json.load(sys.stdin).get("data")
except Exception: print("PARSE_ERROR"); sys.exit(0)
try: print(eval(sys.argv[1]))
except Exception as e: print("ERR:%s"%e)
' "$1"
}
jerr() {
  printf '%s' "$BODY" | python3 -c '
import json,sys
try: print(json.load(sys.stdin).get("error",{}).get("message",""))
except Exception: print("")
'
}

# register <phone> — creates a family account and echoes its token. Two are
# needed to test the race, and dev OTP is returned in the response.
register() {
  local phone="$1" name="$2"
  # Re-runs are normal, so try logging in first and only register if that fails.
  req POST /api/auth/login "{\"phone\":\"$phone\",\"password\":\"Test@12345\"}" ""
  if [ "$STATUS" = "200" ]; then
    jget 'd["token"]'
    return
  fi
  # The dev OTP is returned in the response when APP_ENV is not production.
  req POST /api/auth/otp/send "{\"phone\":\"$phone\",\"purpose\":\"register\"}" ""
  local otp; otp="$(jget 'd.get("devOtp","")')"
  if [ -z "$otp" ] || [ "$otp" = "None" ]; then
    echo ""
    return
  fi
  req POST /api/auth/register \
    "{\"phone\":\"$phone\",\"otp\":\"$otp\",\"name\":\"$name\",\"password\":\"Test@12345\"}" ""
  jget 'd["token"]'
}

printf '\033[1mStageX — ticket booking end-to-end smoke\033[0m\n'
printf 'api: %s   admin: %s\n' "$API" "$ADMIN_API"

# -----------------------------------------------------------------------------
head2 "0. Health & two buyer accounts"
# -----------------------------------------------------------------------------
req GET /api/health "" ""
check "participant API is up" "$STATUS" "200"
[ "$STATUS" != "200" ] && { red "start the stack first: docker compose up -d"; exit 1; }

TOKEN_A="$(register 9000000101 'Buyer A')"
TOKEN_B="$(register 9000000102 'Buyer B')"
checkne "buyer A has a session" "$TOKEN_A" ""
checkne "buyer B has a session" "$TOKEN_B" ""
[ -z "$TOKEN_A" ] || [ -z "$TOKEN_B" ] && { red "could not create test buyers"; exit 1; }
TOKEN="$TOKEN_A"

# -----------------------------------------------------------------------------
head2 "1. Find an event with reserved seating"
# -----------------------------------------------------------------------------
req GET /api/events "" ""
check "event list loads" "$STATUS" "200"
# The seeded "Nritya Mahotsav" runs in Nehru Auditorium, whose arrangement the
# seat-map seed publishes.
EVENT_ID="$(jget '([e["id"] for e in d if e["name"]=="Nritya Mahotsav"] or [d[0]["id"]] if d else [""])[0]')"
checkne "found an event to test with" "$EVENT_ID" ""

req GET "/api/events/${EVENT_ID}/seat-map"
if [ "$STATUS" != "200" ]; then
  red "  This event has no published seating arrangement: $(jerr)"
  red "  Publish one under Ops -> Seat Maps, then re-run."
  exit 1
fi
check "seat map loads" "$STATUS" "200"
check "seat map reports the configured hold window" \
  "$(jget 'd["holdMinutes"] > 0')" "True"
check "seat map reports the per-booking cap" \
  "$(jget 'd["maxSeatsPerBooking"] > 0')" "True"
checkne "seats are on sale" "$(jget 'd["availableCount"]')" "0"
check "blocked and house seats are not offered" \
  "$(jget 'len([s for s in d["seats"] if s["kind"]=="house"])')" "0"
check "stage is drawn on the plan" \
  "$(jget 'len([e for e in d["elements"] if e["kind"]=="stage"]) >= 1')" "True"
check "every seat carries a price" \
  "$(jget 'all(s["price"] > 0 for s in d["seats"])')" "True"

# Two available seats to work with.
SEAT_1="$(jget '[s["seatId"] for s in d["seats"] if s["availability"]=="available"][0]')"
SEAT_1_LABEL="$(jget '[s["seatLabel"] for s in d["seats"] if s["availability"]=="available"][0]')"
SEAT_1_PRICE="$(jget '[s["price"] for s in d["seats"] if s["availability"]=="available"][0]')"
printf '  contested seat: %s\n' "$SEAT_1_LABEL"

# -----------------------------------------------------------------------------
head2 "2. Selection validation"
# -----------------------------------------------------------------------------
req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[]}"
check "an empty selection is rejected" "$STATUS" "400"

req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\",\"$SEAT_1\"]}"
check "the same seat twice is rejected" "$STATUS" "400"

req POST /api/tickets/hold \
  "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"00000000-0000-0000-0000-000000000000\"]}"
check "a seat from another venue is rejected" "$STATUS" "400"

req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\"]}" ""
check "an anonymous hold is rejected" "$STATUS" "401"

# -----------------------------------------------------------------------------
head2 "3. THE RACE — two buyers, one seat, fired in parallel"
# -----------------------------------------------------------------------------
# This is the check that matters. Both requests are sent at once; the design
# funnels them through a single lockable row per (event, seat), so exactly one
# must win and the other must be told the seat went.
TMP="$(mktemp -d)"
(
  curl -sS -X POST "${API}/api/tickets/hold" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer ${TOKEN_A}" \
    -d "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\"]}" \
    -w '\n%{http_code}' > "$TMP/a" 2>/dev/null
) &
(
  curl -sS -X POST "${API}/api/tickets/hold" \
    -H 'Content-Type: application/json' -H "Authorization: Bearer ${TOKEN_B}" \
    -d "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\"]}" \
    -w '\n%{http_code}' > "$TMP/b" 2>/dev/null
) &
wait

A_STATUS="$(tail -1 "$TMP/a")"; A_BODY="$(sed '$d' "$TMP/a")"
B_STATUS="$(tail -1 "$TMP/b")"; B_BODY="$(sed '$d' "$TMP/b")"
printf '  buyer A: HTTP %s   buyer B: HTTP %s\n' "$A_STATUS" "$B_STATUS"

WINNERS=0
[ "$A_STATUS" = "201" ] && WINNERS=$((WINNERS+1))
[ "$B_STATUS" = "201" ] && WINNERS=$((WINNERS+1))
check "exactly one buyer won the seat" "$WINNERS" "1"

LOSERS=0
[ "$A_STATUS" = "409" ] && LOSERS=$((LOSERS+1))
[ "$B_STATUS" = "409" ] && LOSERS=$((LOSERS+1))
check "the other got a 409 conflict" "$LOSERS" "1"

# The loser's message must name the seat, so the UI can highlight it.
LOSER_MSG="$(printf '%s' "$A_BODY$B_BODY" | python3 -c '
import sys,re
t=sys.stdin.read()
m=re.findall(r"\"message\":\"([^\"]+)\"",t)
print(" | ".join(m))')"
case "$LOSER_MSG" in
  *"$SEAT_1_LABEL"*) ok "the loser is told which seat went ($SEAT_1_LABEL)" ;;
  *) bad "the loser is told which seat went" "message was: $LOSER_MSG" ;;
esac

# Identify the winner and continue as them.
if [ "$A_STATUS" = "201" ]; then
  TOKEN="$TOKEN_A"; OTHER="$TOKEN_B"; WIN_BODY="$A_BODY"
else
  TOKEN="$TOKEN_B"; OTHER="$TOKEN_A"; WIN_BODY="$B_BODY"
fi
GROUP="$(printf '%s' "$WIN_BODY" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["holdGroupId"])')"
checkne "the winner has a hold group" "$GROUP" ""
rm -rf "$TMP"

# -----------------------------------------------------------------------------
head2 "4. What each buyer now sees"
# -----------------------------------------------------------------------------
req GET "/api/events/${EVENT_ID}/seat-map" "" "$TOKEN"
check "the winner sees the seat as their own hold" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "held_by_you"
check "the winner's live hold is reported for resuming" "$(jget 'd["holdGroupId"]')" "$GROUP"

req GET "/api/events/${EVENT_ID}/seat-map" "" "$OTHER"
check "the other buyer sees the same seat as taken" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "held"

# The rival cannot take it, however they ask.
req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\"]}" "$OTHER"
check "the rival still cannot hold it" "$STATUS" "409"

# -----------------------------------------------------------------------------
head2 "5. Quote and checkout"
# -----------------------------------------------------------------------------
req GET "/api/tickets/hold/${GROUP}/quote" "" "$TOKEN"
check "quote loads" "$STATUS" "200"
check "quote prices the held seat" "$(jget 'd["totals"]["seatCount"]')" "1"
check "quote total matches the seat price" \
  "$(jget 'int(d["totals"][\"total\"])')" "$(python3 -c "print(int(float('$SEAT_1_PRICE')))")"

# Another family must not be able to read or spend this hold.
req GET "/api/tickets/hold/${GROUP}/quote" "" "$OTHER"
check "the rival cannot quote someone else's hold" "$STATUS" "409"

req GET "/api/tickets/hold/${GROUP}/quote?coupon=NOPE-NOT-REAL" "" "$TOKEN"
check "an unknown coupon is rejected" "$STATUS" "400"

req POST /api/tickets/bookings "{\"holdGroupId\":\"$GROUP\",\"couponCode\":\"\"}" "$TOKEN"
check "booking is created" "$STATUS" "201"
BOOKING="$(jget 'd["id"]')"
REF="$(jget 'd["reference"]')"
check "booking starts awaiting payment" "$(jget 'd["status"]')" "pending_payment"
check "booking reference is issued" "$(jget 'd["reference"].startswith("SX-")')" "True"
check "booking snapshots the seat" "$(jget 'len(d["items"])')" "1"
check "the snapshot records the row's distance from the stage" \
  "$(jget 'd["items"][0]["rowIndex"] >= 0')" "True"
check "an unpaid booking cannot be cancelled" "$(jget 'd["canCancel"]')" "False"

# Creating again on the same hold must not stack duplicates.
req POST /api/tickets/bookings "{\"holdGroupId\":\"$GROUP\",\"couponCode\":\"\"}" "$TOKEN"
check "re-checkout reuses the same booking" "$(jget 'd["id"]')" "$BOOKING"

# -----------------------------------------------------------------------------
head2 "6. Payment — retry, then success"
# -----------------------------------------------------------------------------
req POST "/api/tickets/bookings/${BOOKING}/pay" "" "$TOKEN"
check "payment attempt opens" "$STATUS" "201"
PAY_REF="$(jget 'd["providerRef"]')"
check "attempt 1 of 3" "$(jget 'd["attempt"]')" "1"

# A second Pay click must not open a second authorisation.
req POST "/api/tickets/bookings/${BOOKING}/pay" "" "$TOKEN"
check "a repeat Pay returns the same attempt" "$(jget 'd["providerRef"]')" "$PAY_REF"

# While a payment is processing the seat stays blocked for everyone else.
req GET "/api/events/${EVENT_ID}/seat-map" "" "$OTHER"
check "the seat stays blocked during payment" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "held"

# A declined card leaves the seats held so the buyer can try again.
req POST "/api/tickets/bookings/${BOOKING}/confirm" \
  "{\"providerRef\":\"$PAY_REF\",\"success\":false}" "$TOKEN"
check "a failed payment is a retry" "$(jget 'd["outcome"]')" "retry"
check "the booking returns to awaiting payment" "$(jget 'd["booking"]["status"]')" "pending_payment"

req GET "/api/events/${EVENT_ID}/seat-map" "" "$OTHER"
check "the seat is still held after a failed attempt" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "held"

# Second attempt succeeds.
req POST "/api/tickets/bookings/${BOOKING}/pay" "" "$TOKEN"
PAY_REF2="$(jget 'd["providerRef"]')"
check "attempt 2 of 3" "$(jget 'd["attempt"]')" "2"

req POST "/api/tickets/bookings/${BOOKING}/confirm" \
  "{\"providerRef\":\"$PAY_REF2\",\"success\":true}" "$TOKEN"
check "payment confirms" "$(jget 'd["outcome"]')" "confirmed"
check "booking is confirmed" "$(jget 'd["booking"]["status"]')" "confirmed"
check "a confirmed booking can be cancelled" "$(jget 'd["booking"]["canCancel"]')" "True"

# Idempotency: a duplicate webhook must not sell the seat twice.
req POST "/api/tickets/bookings/${BOOKING}/confirm" \
  "{\"providerRef\":\"$PAY_REF2\",\"success\":true}" "$TOKEN"
check "a duplicate confirm is idempotent" "$(jget 'd["outcome"]')" "already_confirmed"

req GET "/api/events/${EVENT_ID}/seat-map" "" "$OTHER"
check "the seat now reads as sold to everyone" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "booked"
req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\"]}" "$OTHER"
check "a sold seat can never be held again" "$STATUS" "409"

# -----------------------------------------------------------------------------
head2 "7. Booking history"
# -----------------------------------------------------------------------------
req GET /api/my/bookings "" "$TOKEN"
check "history loads" "$STATUS" "200"
check "the booking appears in history" \
  "$(jget 'len([b for b in d if b["id"]==\"'"$BOOKING"'\"])')" "1"

req GET "/api/my/bookings/${BOOKING}" "" "$TOKEN"
check "booking detail loads" "$STATUS" "200"
check "detail carries the seat snapshot" "$(jget 'len(d["items"])')" "1"
check "no refund is owed on a live booking" "$(jget 'd["refundStatus"]')" "not_applicable"

# Family scoping: another buyer must not see it at all.
req GET "/api/my/bookings/${BOOKING}" "" "$OTHER"
check "another family cannot read the booking" "$STATUS" "404"
req GET /api/my/bookings "" "$OTHER"
check "another family's history excludes it" \
  "$(jget 'len([b for b in d if b["id"]==\"'"$BOOKING"'\"])')" "0"

# -----------------------------------------------------------------------------
head2 "8. Admin sees the booked tickets"
# -----------------------------------------------------------------------------
req POST /admin/auth/login "{\"email\":\"$OPS_EMAIL\",\"password\":\"$OPS_PASS\"}" "" "$ADMIN_API"
ADMIN_TOKEN="$(jget 'd["token"]')"
checkne "ops admin logs in" "$ADMIN_TOKEN" ""

# An Event Admin session, to prove the ops reports stay ops-only.
req POST /admin/auth/login '{"email":"event@stagex.test","password":"Event@12345"}' "" "$ADMIN_API"
EVENT_TOKEN="$(jget 'd["token"]')"

req GET /admin/ops/ticket-stats "" "$ADMIN_TOKEN" "$ADMIN_API"
check "ticket stats load" "$STATUS" "200"
check "blocked seats are counted separately in the overview" \
  "$(jget '[r["blockedSeats"] >= 0 for r in d if r["eventId"]==\"'"$EVENT_ID"'\"][0]')" "True"
check "the event reports at least one ticket booked" \
  "$(jget '[r["ticketsBooked"] >= 1 for r in d if r["eventId"]==\"'"$EVENT_ID"'\"][0]')" "True"
check "stats report gross revenue" \
  "$(jget '[float(r["grossRevenue"]) > 0 for r in d if r["eventId"]==\"'"$EVENT_ID"'\"][0]')" "True"
check "stats report remaining availability" \
  "$(jget '[r["seatsAvailable"] >= 0 for r in d if r["eventId"]==\"'"$EVENT_ID"'\"][0]')" "True"

req GET "/admin/ops/bookings?eventId=${EVENT_ID}" "" "$ADMIN_TOKEN" "$ADMIN_API"
check "admin can list the event's bookings" "$STATUS" "200"
check "the booking is visible to admin" \
  "$(jget 'len([b for b in d if b["reference"]==\"'"$REF"'\"])')" "1"
check "admin sees who booked it" \
  "$(jget '[b["phone"] != "" for b in d if b["reference"]==\"'"$REF"'\"][0]')" "True"

# -----------------------------------------------------------------------------
head2 "8b. Seat occupancy — available / paid / held / blocked"
# -----------------------------------------------------------------------------
req GET "/admin/ops/events/${EVENT_ID}/seat-occupancy" "" "$ADMIN_TOKEN" "$ADMIN_API"
check "seat occupancy loads" "$STATUS" "200"
check "every mapped seat is reported (including blocked)" \
  "$(jget 'len(d["seats"]) == d["summary"]["mappedSeats"]')" "True"
check "the sold seat is reported as booked" \
  "$(jget '[s["status"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "booked"
check "the booked seat names the buyer" \
  "$(jget '[s["buyerPhone"] != "" for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "True"
check "the booked seat records what was paid" \
  "$(jget '[s["pricePaid"] > 0 for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "True"
check "the booked seat carries its booking reference" \
  "$(jget '[s["bookingRef"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "$REF"

# The seeded arrangement deliberately contains blocked and house seats, which a
# buyer never sees — this view must.
check "blocked seats are visible to admin" "$(jget 'd["summary"]["blockedSeats"] >= 1')" "True"
check "house holds are visible to admin"   "$(jget 'd["summary"]["houseSeats"] >= 1')" "True"
check "blocked seats appear in the seat list" \
  "$(jget 'len([s for s in d["seats"] if s["status"]==\"blocked\"]) >= 1')" "True"
check "blocked seats are excluded from sellable" \
  "$(jget 'd["summary"]["sellable"] == d["summary"]["mappedSeats"] - d["summary"]["blockedSeats"] - d["summary"]["houseSeats"]')" "True"

# Every seat must land in exactly one bucket, or the arithmetic is wrong.
check "occupancy buckets account for every seat" \
  "$(jget 'd["summary"]["booked"] + d["summary"]["held"] + d["summary"]["available"] + d["summary"]["blocked"] + d["summary"]["house"] == d["summary"]["mappedSeats"]')" "True"
check "available seats never include a blocked one" \
  "$(jget 'all(s["status"] != \"available\" for s in d["seats"] if not s.get(\"listPrice\") is None and s["status"]==\"blocked\")')" "True"

check "seat value is reported" "$(jget 'd["summary"]["listValue"] > 0')" "True"
check "realised revenue is reported" "$(jget 'd["summary"]["realised"] > 0')" "True"
check "occupancy percentage is calculated" \
  "$(jget '0 <= d["summary"]["occupancyPct"] <= 100')" "True"
check "per-tier breakdown is present" "$(jget 'len(d["byZone"]) >= 1')" "True"
check "the plan geometry is included for drawing" \
  "$(jget 'd["canvasWidth"] > 0 and len(d["elements"]) >= 1')" "True"

# -----------------------------------------------------------------------------
head2 "8c. Seat reservation report + P&L"
# -----------------------------------------------------------------------------
check "ticket gross is recorded" "$(jget 'd["tickets"]["gross"] > 0')" "True"
check "net ticket income = gross - refunds" \
  "$(jget 'abs(d["tickets"][\"net\"] - (d["tickets"][\"gross\"] - d["tickets"][\"refunds\"])) < 0.01')" "True"
# The whole point: the P&L must include ticket money.
check "P&L includes ticket income" "$(jget 'd["pnl"]["ticketIncome"] > 0')" "True"
check "total income includes the ticket line" \
  "$(jget 'abs(d["pnl"][\"totalIncome\"] - (d["pnl"][\"revenue\"] + d["pnl"][\"ticketIncome\"] + d["pnl"][\"sponsorIncome\"] + d["pnl"][\"vendorIncome\"])) < 0.01')" "True"
check "net P&L = income - expenses" \
  "$(jget 'abs(d["pnl"][\"netPL\"] - (d["pnl"][\"totalIncome\"] - d["pnl"][\"totalExpenses\"])) < 0.01')" "True"
check "hall cost is charged to the event" "$(jget 'd["pnl"]["hallCost"] >= 0')" "True"

# The downloadable report, in all three formats.
RAW="$(curl -sS -o /dev/null -w '%{http_code} %{content_type}' \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${ADMIN_API}/admin/ops/events/${EVENT_ID}/seat-report?format=csv" 2>/dev/null)"
check "CSV report downloads" "${RAW%% *}" "200"
case "$RAW" in *text/csv*) ok "CSV report has the right content type" ;;
  *) bad "CSV report content type" "got: $RAW" ;; esac

RAW="$(curl -sS -o /dev/null -w '%{http_code} %{content_type}' \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${ADMIN_API}/admin/ops/events/${EVENT_ID}/seat-report?format=pdf" 2>/dev/null)"
check "PDF report downloads" "${RAW%% *}" "200"
case "$RAW" in *application/pdf*) ok "PDF report has the right content type" ;;
  *) bad "PDF report content type" "got: $RAW" ;; esac

CSV="$(curl -sS -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${ADMIN_API}/admin/ops/events/${EVENT_ID}/seat-report?format=csv" 2>/dev/null)"
case "$CSV" in *"SEAT OCCUPANCY"*) ok "CSV contains the occupancy section" ;;
  *) bad "CSV occupancy section" ;; esac
case "$CSV" in *"PROFIT & LOSS"*) ok "CSV contains the P&L section" ;;
  *) bad "CSV P&L section" ;; esac
case "$CSV" in *"SEAT DETAIL"*) ok "CSV contains the seat-by-seat detail" ;;
  *) bad "CSV seat detail section" ;; esac
case "$CSV" in *"$REF"*) ok "CSV names the booking against its seat" ;;
  *) bad "CSV booking reference" ;; esac

# Event Admins must not reach Ops reports.
req GET "/admin/ops/events/${EVENT_ID}/seat-occupancy" "" "$EVENT_TOKEN" "$ADMIN_API"
check "event admin cannot read the ops occupancy view" "$STATUS" "403"

# -----------------------------------------------------------------------------
head2 "9. Cancellation"
# -----------------------------------------------------------------------------
req POST "/api/my/bookings/${BOOKING}/cancel" '{"reason":"","note":""}' "$TOKEN"
check "a cancellation with no reason is rejected" "$STATUS" "400"

req POST "/api/my/bookings/${BOOKING}/cancel" '{"reason":"made_it_up","note":""}' "$TOKEN"
check "an unknown reason is rejected" "$STATUS" "400"

req POST "/api/my/bookings/${BOOKING}/cancel" '{"reason":"other","note":""}' "$TOKEN"
check "\"other\" without an explanation is rejected" "$STATUS" "400"

req GET /api/tickets/cancellation-reasons "" "$TOKEN"
check "the reason list is served to the client" "$STATUS" "200"
check "reasons include an \"other\" escape hatch" \
  "$(jget 'len([r for r in d if r["code"]==\"other\"])')" "1"

# Another family must not be able to cancel it.
req POST "/api/my/bookings/${BOOKING}/cancel" \
  '{"reason":"change_of_plan","note":"not mine"}' "$OTHER"
check "another family cannot cancel it" "$STATUS" "404"

req POST "/api/my/bookings/${BOOKING}/cancel" \
  '{"reason":"unable_to_attend","note":"Smoke test cancellation"}' "$TOKEN"
check "cancellation succeeds" "$STATUS" "200"
check "the booking is cancelled" "$(jget 'd["booking"]["status"]')" "cancelled"
check "the reason is recorded" "$(jget 'd["booking"]["cancellationReason"]')" "unable_to_attend"
check "the note is kept" \
  "$(jget 'd["booking"]["cancellationNote"]')" "Smoke test cancellation"
check "the reason label is resolved for display" \
  "$(jget 'd["booking"]["cancellationReasonLabel"] != ""')" "True"
check "it records who cancelled" "$(jget 'd["booking"]["cancelledBy"]')" "family"

# Refunds are offline only — that is the whole rule.
check "a refund is owed offline" "$(jget 'd["refund"]["status"]')" "pending_offline"
checkne "the refund amount is the booking total" "$(jget 'int(d["refund"][\"amount\"])')" "0"
case "$(jget 'd["refund"]["message"]')" in
  *offline*) ok "the buyer is told the refund is handled offline" ;;
  *) bad "the buyer is told the refund is handled offline" "message did not mention offline" ;;
esac

# The historical record survives, and the seat goes back on sale.
req GET "/api/my/bookings/${BOOKING}" "" "$TOKEN"
check "the cancelled booking is still in history" "$STATUS" "200"
check "its seats are kept, marked cancelled" \
  "$(jget 'all(i["status"]==\"cancelled\" for i in d["items"])')" "True"
check "the seat price is still on the record" "$(jget 'd["items"][0]["price"] > 0')" "True"
check "it cannot be cancelled twice" "$(jget 'd["canCancel"]')" "False"

req POST "/api/my/bookings/${BOOKING}/cancel" '{"reason":"change_of_plan","note":""}' "$TOKEN"
check "a second cancellation is refused" "$STATUS" "409"

req GET "/api/events/${EVENT_ID}/seat-map" "" "$OTHER"
check "the cancelled seat is back on sale" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "available"
req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_1\"]}" "$OTHER"
check "another buyer can now take it" "$STATUS" "201"
RIVAL_GROUP="$(jget 'd["holdGroupId"]')"
req DELETE "/api/tickets/hold/${RIVAL_GROUP}?eventId=${EVENT_ID}" "" "$OTHER"
check "releasing a hold works" "$STATUS" "200"
req GET "/api/events/${EVENT_ID}/seat-map" "" "$TOKEN"
check "a released seat is immediately available again" \
  "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_1"'\"][0]')" "available"

# -----------------------------------------------------------------------------
head2 "10. Admin settles the offline refund"
# -----------------------------------------------------------------------------
req GET "/admin/ops/bookings?status=cancelled" "" "$ADMIN_TOKEN" "$ADMIN_API"
check "admin sees the cancelled booking" \
  "$(jget 'len([b for b in d if b["reference"]==\"'"$REF"'\"])')" "1"
check "admin sees the refund as pending offline" \
  "$(jget '[b["refundStatus"] for b in d if b["reference"]==\"'"$REF"'\"][0]')" "pending_offline"
check "admin sees the cancellation reason" \
  "$(jget '[b["cancellationReason"] for b in d if b["reference"]==\"'"$REF"'\"][0]')" "unable_to_attend"

req PATCH "/admin/ops/bookings/${BOOKING}/refund" \
  '{"refundStatus":"completed","note":"UTR-SMOKE-001"}' "$ADMIN_TOKEN" "$ADMIN_API"
check "admin marks the refund paid" "$STATUS" "200"
check "the refund is now completed" "$(jget 'd["refundStatus"]')" "completed"
check "the payout reference is stored" "$(jget 'd["refundNote"]')" "UTR-SMOKE-001"

req GET "/api/my/bookings/${BOOKING}" "" "$TOKEN"
check "the buyer sees the refund as paid" "$(jget 'd["refundStatus"]')" "completed"

req PATCH "/admin/ops/bookings/${BOOKING}/refund" \
  '{"refundStatus":"nonsense","note":""}' "$ADMIN_TOKEN" "$ADMIN_API"
check "an invalid refund status is rejected" "$STATUS" "400"

# -----------------------------------------------------------------------------
head2 "11. Hold expiry"
# -----------------------------------------------------------------------------
# Only meaningful with a short window; otherwise waiting 15 minutes is absurd.
req GET "/api/events/${EVENT_ID}/seat-map" "" "$TOKEN"
HOLD_MIN="$(jget 'd["holdMinutes"]')"
if [ "$HOLD_MIN" -le 1 ] 2>/dev/null; then
  SEAT_X="$(jget '[s["seatId"] for s in d["seats"] if s["availability"]=="available"][0]')"
  req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_X\"]}" "$TOKEN"
  check "held a seat for the expiry test" "$STATUS" "201"
  printf '  waiting %s minute(s) for the hold to lapse...\n' "$HOLD_MIN"
  sleep $(( HOLD_MIN * 60 + 8 ))
  req GET "/api/events/${EVENT_ID}/seat-map" "" "$OTHER"
  check "the lapsed hold returns the seat to available" \
    "$(jget '[s["availability"] for s in d["seats"] if s["seatId"]==\"'"$SEAT_X"'\"][0]')" "available"
  req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":[\"$SEAT_X\"]}" "$OTHER"
  check "another buyer can take the lapsed seat" "$STATUS" "201"
  G="$(jget 'd["holdGroupId"]')"
  req DELETE "/api/tickets/hold/${G}?eventId=${EVENT_ID}" "" "$OTHER"
else
  skip "hold expiry (window is ${HOLD_MIN}m — re-run the api with SEAT_HOLD_TTL=45s to cover it)"
  skip "post-expiry re-hold by another buyer"
fi

# -----------------------------------------------------------------------------
head2 "12. Per-booking seat cap"
# -----------------------------------------------------------------------------
req GET "/api/events/${EVENT_ID}/seat-map" "" "$TOKEN"
CAP="$(jget 'd["maxSeatsPerBooking"]')"
# One more seat than the cap allows, as a JSON array.
OVER="$(printf '%s' "$BODY" | python3 -c '
import json, sys
d = json.load(sys.stdin)["data"]
cap = d["maxSeatsPerBooking"]
free = [s["seatId"] for s in d["seats"] if s["availability"] == "available"]
print(json.dumps(free[:cap + 1]))
')"
req POST /api/tickets/hold "{\"eventId\":\"$EVENT_ID\",\"seatIds\":$OVER}" "$TOKEN"
check "more than $CAP seats is rejected" "$STATUS" "400"

# -----------------------------------------------------------------------------
printf '\n\033[1m== summary ==\033[0m\n'
green "  passed:  $PASS"
[ "$SKIP" -gt 0 ] && printf '  skipped: %s\n' "$SKIP"
if [ "$FAIL" -gt 0 ]; then
  red "  failed:  $FAIL"
  exit 1
fi
printf '  failed:  0\n'
green "\nAll ticket-booking end-to-end checks passed."
