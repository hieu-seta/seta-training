#!/usr/bin/env bash
# Phase-03 e2e: asset-svc on :8083.
# Flow: register 3 users (owner + sharee + manager) → create folder → create note → share → owner/sharee/mgr ACLs → revoke.

set -euo pipefail

AUTH="${AUTH_BASE:-http://localhost:8081}"
TEAM="${TEAM_BASE:-http://localhost:8082}"
ASSET="${ASSET_BASE:-http://localhost:8083}"
PW="password123"
STAMP=$(date +%s)
OWNER_EMAIL="own+$STAMP@example.com"
SHAR_EMAIL="shr+$STAMP@example.com"
MGR_EMAIL="mgr+$STAMP@example.com"

say()  { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

say "register owner (member), sharee (member), mgr (manager)"
owner=$(curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' -d "{\"username\":\"o\",\"email\":\"$OWNER_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}")
shar=$(curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' -d "{\"username\":\"s\",\"email\":\"$SHAR_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}")
mgr=$(curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' -d "{\"username\":\"m\",\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\",\"role\":\"manager\"}")
owner_uid=$(echo "$owner" | json "['id']")
shar_uid=$(echo "$shar" | json "['id']")
mgr_uid=$(echo "$mgr" | json "['id']")

owner_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' -d "{\"email\":\"$OWNER_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
shar_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' -d "{\"email\":\"$SHAR_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
mgr_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' -d "{\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\"}" | json "['access']")

say "mgr creates team, adds owner as member"
team=$(curl -s -X POST "$TEAM/teams" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d '{"name":"T1"}')
team_id=$(echo "$team" | json "['id']")
[ -n "$team_id" ] || fail "team create failed"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$TEAM/teams/$team_id/members" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d "{\"user_id\":\"$owner_uid\"}")
[ "$code" = "204" ] || fail "add member failed: $code"

say "owner creates folder → 201"
folder=$(curl -s -X POST "$ASSET/folders" -H 'Content-Type: application/json' -H "Authorization: Bearer $owner_tok" -d '{"name":"FolderA"}')
folder_id=$(echo "$folder" | json "['id']")
[ -n "$folder_id" ] || fail "create folder: $folder"

say "owner creates note → 201"
note=$(curl -s -X POST "$ASSET/folders/$folder_id/notes" -H 'Content-Type: application/json' -H "Authorization: Bearer $owner_tok" -d '{"title":"hello","body":"world"}')
note_id=$(echo "$note" | json "['id']")
[ -n "$note_id" ] || fail "create note"

say "sharee cannot read note (no share) → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $shar_tok" "$ASSET/notes/$note_id")
[ "$code" = "403" ] || fail "sharee unexpected $code"

say "mgr (oversight) reads note → 200"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $mgr_tok" "$ASSET/notes/$note_id")
[ "$code" = "200" ] || fail "mgr read got $code"

say "mgr cannot write note → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$ASSET/notes/$note_id" -H 'Content-Type: application/json' -H "Authorization: Bearer $mgr_tok" -d '{"title":"tamper","body":""}')
[ "$code" = "403" ] || fail "mgr write expected 403, got $code"

say "owner shares FOLDER w/ sharee read → 204"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$ASSET/folders/$folder_id/share" -H 'Content-Type: application/json' -H "Authorization: Bearer $owner_tok" -d "{\"user_id\":\"$shar_uid\",\"access\":\"read\"}")
[ "$code" = "204" ] || fail "share got $code"

say "sharee reads note via folder inheritance → 200"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $shar_tok" "$ASSET/notes/$note_id")
[ "$code" = "200" ] || fail "sharee inherited read got $code"

say "sharee cannot write note (read-only share) → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$ASSET/notes/$note_id" -H 'Content-Type: application/json' -H "Authorization: Bearer $shar_tok" -d '{"title":"new","body":""}')
[ "$code" = "403" ] || fail "read-share write got $code"

say "upgrade share to write → 204"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$ASSET/folders/$folder_id/share" -H 'Content-Type: application/json' -H "Authorization: Bearer $owner_tok" -d "{\"user_id\":\"$shar_uid\",\"access\":\"write\"}")
[ "$code" = "204" ] || fail "upgrade share got $code"

say "sharee writes note → 200"
code=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$ASSET/notes/$note_id" -H 'Content-Type: application/json' -H "Authorization: Bearer $shar_tok" -d '{"title":"upd","body":"body2"}')
[ "$code" = "200" ] || fail "write after upgrade got $code"

say "revoke share → 204"
code=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$ASSET/folders/$folder_id/share/$shar_uid" -H "Authorization: Bearer $owner_tok")
[ "$code" = "204" ] || fail "revoke got $code"

say "sharee read after revoke → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $shar_tok" "$ASSET/notes/$note_id")
[ "$code" = "403" ] || fail "post-revoke read got $code"

say "sharee cannot share folder (not owner) → 403"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$ASSET/folders/$folder_id/share" -H 'Content-Type: application/json' -H "Authorization: Bearer $shar_tok" -d "{\"user_id\":\"$mgr_uid\",\"access\":\"read\"}")
[ "$code" = "403" ] || fail "sharee share-as-nonowner got $code"

say "no JWT → 401"
code=$(curl -s -o /dev/null -w "%{http_code}" "$ASSET/folders")
[ "$code" = "401" ] || fail "no-jwt got $code"

say "list folders as owner returns FolderA"
list=$(curl -s -H "Authorization: Bearer $owner_tok" "$ASSET/folders")
echo "$list" | grep -q "$folder_id" || fail "list missing folder"

printf "\033[32m✓ phase-03 e2e PASS\033[0m\n"
