#!/usr/bin/env bash
# Phase-02 e2e: drive team-svc on :8082 through auth-svc on :8081.
# Flow: register manager + member → login both → create team → add member → list → detail → role gates.

set -euo pipefail

AUTH="${AUTH_BASE:-http://localhost:8081}"
TEAM="${TEAM_BASE:-http://localhost:8082}"
PW="password123"
STAMP=$(date +%s)
MGR_EMAIL="mgr+$STAMP@example.com"
MEM_EMAIL="mem+$STAMP@example.com"

say()  { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

say "register manager + member"
mgr=$(curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"m\",\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\",\"role\":\"manager\"}")
mem=$(curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"u\",\"email\":\"$MEM_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}")
mgr_uid=$(echo "$mgr" | json "['id']")
mem_uid=$(echo "$mem" | json "['id']")
[ -n "$mgr_uid" ] && [ -n "$mem_uid" ] || fail "register failed"

say "login both"
mgr_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
mem_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$MEM_EMAIL\",\"password\":\"$PW\"}" | json "['access']")

say "create team as manager → 201"
team=$(curl -s -X POST "$TEAM/teams" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $mgr_tok" -d '{"name":"Engineering"}')
team_id=$(echo "$team" | json "['id']")
[ -n "$team_id" ] || fail "create team failed: $team"

say "create team as member → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $mem_tok" \
  -d '{"name":"NopeTeam"}')
[ "$code" = "403" ] || fail "expected 403, got $code"

say "create team no JWT → 401"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams" \
  -H 'Content-Type: application/json' -d '{"name":"X"}')
[ "$code" = "401" ] || fail "expected 401, got $code"

say "add member to team → 204"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams/$team_id/members" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" \
  -d "{\"user_id\":\"$mem_uid\"}")
[ "$code" = "204" ] || fail "expected 204, got $code"

say "add member nonexistent → 404"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams/$team_id/members" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" \
  -d '{"user_id":"00000000-0000-0000-0000-000000000000"}')
[ "$code" = "404" ] || fail "expected 404, got $code"

say "list teams (authd) → 200"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $mgr_tok" "$TEAM/teams")
[ "$code" = "200" ] || fail "expected 200, got $code"

say "detail as member → 200 (was added)"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $mem_tok" "$TEAM/teams/$team_id")
[ "$code" = "200" ] || fail "expected 200, got $code"

say "remove member → 204"
code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$TEAM/teams/$team_id/members/$mem_uid" \
  -H "Authorization: Bearer $mgr_tok")
[ "$code" = "204" ] || fail "expected 204, got $code"

say "detail as ex-member → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $mem_tok" "$TEAM/teams/$team_id")
[ "$code" = "403" ] || fail "expected 403, got $code"

say "add manager (mgr is main) → 204"
# register 3rd user to promote
PROMO_EMAIL="promo+$STAMP@example.com"
promo=$(curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"p\",\"email\":\"$PROMO_EMAIL\",\"password\":\"$PW\",\"role\":\"manager\"}")
promo_uid=$(echo "$promo" | json "['id']")
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams/$team_id/managers" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" \
  -d "{\"user_id\":\"$promo_uid\"}")
[ "$code" = "204" ] || fail "expected 204 add mgr, got $code"

say "promoted mgr cannot add another mgr (not main) → 403"
promo_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$PROMO_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams/$team_id/managers" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $promo_tok" \
  -d "{\"user_id\":\"$mgr_uid\"}")
[ "$code" = "403" ] || fail "expected 403 from non-main mgr, got $code"

printf "\033[32m✓ phase-02 e2e PASS\033[0m\n"
