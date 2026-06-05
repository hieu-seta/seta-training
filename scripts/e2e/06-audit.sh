#!/usr/bin/env bash
# Phase-06 e2e: trigger team + asset mutations, verify audit.events rows landed.

set -euo pipefail
AUTH="${AUTH_BASE:-http://localhost:8081}"
TEAM="${TEAM_BASE:-http://localhost:8082}"
ASSET="${ASSET_BASE:-http://localhost:8083}"
PW="password123"
STAMP=$(date +%s)
MGR_EMAIL="audit+mgr+$STAMP@example.com"
OWN_EMAIL="audit+own+$STAMP@example.com"

say()  { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }
audit_count() {
  docker exec seta-pg psql -U seta -d seta -tA -c "SELECT count(*) FROM audit.events $1"
}

before_total=$(audit_count "")
say "baseline audit count: $before_total"

say "register manager + owner, login"
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"m\",\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\",\"role\":\"manager\"}" >/dev/null
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"o\",\"email\":\"$OWN_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}" >/dev/null
mgr_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
own_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$OWN_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
own_uid=$(curl -s "$AUTH/me" -H "Authorization: Bearer $own_tok" | json "['uid']")
mgr_uid=$(curl -s "$AUTH/me" -H "Authorization: Bearer $mgr_tok" | json "['uid']")

say "trigger 2 ACTIVITY events"
team=$(curl -s -X POST "$TEAM/teams" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d '{"name":"AuditT"}')
team_id=$(echo "$team" | json "['id']")
curl -s -X POST "$TEAM/teams/$team_id/members" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $mgr_tok" -d "{\"user_id\":\"$own_uid\"}" >/dev/null

say "trigger 4 ASSETS events"
folder=$(curl -s -X POST "$ASSET/folders" -H 'Content-Type: application/json' -H "Authorization: Bearer $own_tok" -d '{"name":"AuditF"}')
folder_id=$(echo "$folder" | json "['id']")
note=$(curl -s -X POST "$ASSET/folders/$folder_id/notes" -H 'Content-Type: application/json' -H "Authorization: Bearer $own_tok" -d '{"title":"x","body":"y"}')
note_id=$(echo "$note" | json "['id']")
curl -s -X POST "$ASSET/folders/$folder_id/share" -H 'Content-Type: application/json' -H "Authorization: Bearer $own_tok" -d "{\"user_id\":\"$mgr_uid\",\"access\":\"read\"}" >/dev/null
curl -s -X DELETE "$ASSET/folders/$folder_id/share/$mgr_uid" -H "Authorization: Bearer $own_tok" >/dev/null

# Consumers are async — give them a moment.
sleep 2
after_total=$(audit_count "")
say "post-trigger audit count: $after_total"

delta=$((after_total - before_total))
[ "$delta" -ge 6 ] || fail "expected ≥6 audit rows, got $delta"

say "verify subject mix in last $delta rows"
subj_count=$(docker exec seta-pg psql -U seta -d seta -tA -c "SELECT count(distinct subject) FROM audit.events WHERE occurred_at > now() - interval '30 seconds'")
[ "$subj_count" -ge 4 ] || fail "expected ≥4 distinct subjects, got $subj_count"

say "verify idempotency — audit table has UNIQUE on idem_key (re-trigger same event must dedup)"
# Use note_id as deterministic seed — recreating same note isn't possible via API, so just verify the unique constraint exists.
constraint=$(docker exec seta-pg psql -U seta -d seta -tA -c "SELECT conname FROM pg_constraint WHERE conname LIKE '%idem_key%' LIMIT 1")
[ -n "$constraint" ] || fail "expected unique idem_key constraint"

printf "\033[32m✓ phase-06 e2e PASS — audit pipeline live (delta=%d, distinct subjects=%d)\033[0m\n" "$delta" "$subj_count"
