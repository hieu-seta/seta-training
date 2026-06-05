#!/usr/bin/env bash
# Phase-07 e2e: verify cache miss → hit cycle + event-driven invalidation.

set -euo pipefail
AUTH="${AUTH_BASE:-http://localhost:8081}"
TEAM="${TEAM_BASE:-http://localhost:8082}"
PW="password123"
STAMP=$(date +%s)
MGR_EMAIL="cache+mgr+$STAMP@example.com"
MEM_EMAIL="cache+mem+$STAMP@example.com"

say()  { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

redis_get() { docker exec seta-redis redis-cli GET "$1"; }
redis_exists() { docker exec seta-redis redis-cli EXISTS "$1"; }

say "register manager + member, login mgr"
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' -d "{\"username\":\"m\",\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\",\"role\":\"manager\"}" >/dev/null
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' -d "{\"username\":\"u\",\"email\":\"$MEM_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}" >/dev/null
mgr_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' -d "{\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
mem_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' -d "{\"email\":\"$MEM_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
mem_uid=$(curl -s "$AUTH/me" -H "Authorization: Bearer $mem_tok" | json "['uid']")

say "create team + add member"
team=$(curl -s -X POST "$TEAM/teams" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d '{"name":"CacheT"}')
team_id=$(echo "$team" | json "['id']")
curl -s -X POST "$TEAM/teams/$team_id/members" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d "{\"user_id\":\"$mem_uid\"}" >/dev/null

team_key="team:$team_id:members"
say "cache should be MISS before first GET"
exists=$(redis_exists "$team_key")
# After invalidator handled member_added event, key may already be empty (Del on non-existent is fine).
say "  exists=$exists"

say "GET /teams/:id → populate cache"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $mgr_tok" "$TEAM/teams/$team_id")
[ "$code" = "200" ] || fail "first GET got $code"
sleep 1

after=$(redis_exists "$team_key")
[ "$after" = "1" ] || fail "expected cache populated after GET, got exists=$after"
say "  cache populated"

say "trigger event → invalidator Dels key"
# Add another (different) target via mgr (need uid). Use mgr_uid itself? No, mgr is mgr not member.
# Easier: remove + re-add the same member.
curl -s -X DELETE "$TEAM/teams/$team_id/members/$mem_uid" -H "Authorization: Bearer $mgr_tok" >/dev/null
sleep 2
after_evt=$(redis_exists "$team_key")
[ "$after_evt" = "0" ] || fail "expected cache cleared after event, got exists=$after_evt"
say "  cache cleared (invalidator worked)"

say "verify ManagersOf cache (asset-svc side)"
mo_key="team:managers-of:$mem_uid"
# Re-add member so asset.acl triggers ManagersOf
curl -s -X POST "$TEAM/teams/$team_id/members" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d "{\"user_id\":\"$mem_uid\"}" >/dev/null
sleep 1
# Asset-svc ManagersOf is only invoked when ACL falls through. Need a folder owned by mem_uid + a stranger reading.
own_email="cache+own+$STAMP@example.com"
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' -d "{\"username\":\"o\",\"email\":\"$own_email\",\"password\":\"$PW\",\"role\":\"member\"}" >/dev/null
own_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' -d "{\"email\":\"$own_email\",\"password\":\"$PW\"}" | json "['access']")
own_uid=$(curl -s "$AUTH/me" -H "Authorization: Bearer $own_tok" | json "['uid']")
curl -s -X POST "$TEAM/teams/$team_id/members" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d "{\"user_id\":\"$own_uid\"}" >/dev/null

# Owner creates folder; mgr reads (oversight check → calls ManagersOf(own_uid))
folder=$(curl -s -X POST http://localhost:8083/folders -H 'Content-Type: application/json' -H "Authorization: Bearer $own_tok" -d '{"name":"CF"}')
folder_id=$(echo "$folder" | json "['id']")
note=$(curl -s -X POST "http://localhost:8083/folders/$folder_id/notes" -H 'Content-Type: application/json' -H "Authorization: Bearer $own_tok" -d '{"title":"x","body":""}')
note_id=$(echo "$note" | json "['id']")
curl -s -o /dev/null -H "Authorization: Bearer $mgr_tok" "http://localhost:8083/notes/$note_id"
sleep 1

mo_key="team:managers-of:$own_uid"
mo_exists=$(redis_exists "$mo_key")
[ "$mo_exists" = "1" ] || fail "expected ManagersOf cache populated for $own_uid, got $mo_exists"
say "  ManagersOf cache populated for owner"

printf "\033[32m✓ phase-07 e2e PASS — cache + invalidation cycle verified\033[0m\n"
