#!/usr/bin/env bash
# Phase-04 e2e: POST /import-users w/ 5-row CSV including 1 duplicate → 207 partial-failure.

set -euo pipefail
AUTH="${AUTH_BASE:-http://localhost:8081}"
PW="password123"
STAMP=$(date +%s)
MGR_EMAIL="impmgr+$STAMP@example.com"
DUP_EMAIL="dup+$STAMP@example.com"

say()  { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

say "register importer (manager) + login"
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"m\",\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\",\"role\":\"manager\"}" >/dev/null
mgr_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$MGR_EMAIL\",\"password\":\"$PW\"}" | json "['access']")

say "register existing user to force one duplicate in import"
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"x\",\"email\":\"$DUP_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}" >/dev/null

TMP=$(mktemp -d)
trap "rm -rf $TMP" EXIT
CSV="$TMP/users.csv"
cat > "$CSV" <<EOF
username,email,password,role
imp1,imp1+$STAMP@example.com,password123,member
imp2,imp2+$STAMP@example.com,password123,member
imp3,imp3+$STAMP@example.com,password123,manager
imp4,$DUP_EMAIL,password123,member
imp5,imp5+$STAMP@example.com,password123,member
EOF

say "POST /import-users multipart → 207 (1 dup)"
resp_file="$TMP/resp.json"
code=$(curl -s -o "$resp_file" -w "%{http_code}" -X POST "$AUTH/import-users" \
  -H "Authorization: Bearer $mgr_tok" -F "file=@$CSV;type=text/csv")
[ "$code" = "207" ] || { cat "$resp_file"; fail "expected 207, got $code"; }
processed=$(python3 -c "import json;print(json.load(open('$resp_file'))['processed'])")
failed_count=$(python3 -c "import json;print(len(json.load(open('$resp_file'))['failed']))")
[ "$processed" = "4" ] || fail "processed expected 4, got $processed"
[ "$failed_count" = "1" ] || fail "failed expected 1, got $failed_count"

say "non-manager (member) gets 403"
MEM_EMAIL="impmem+$STAMP@example.com"
curl -s -X POST "$AUTH/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"u\",\"email\":\"$MEM_EMAIL\",\"password\":\"$PW\",\"role\":\"member\"}" >/dev/null
mem_tok=$(curl -s -X POST "$AUTH/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$MEM_EMAIL\",\"password\":\"$PW\"}" | json "['access']")
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$AUTH/import-users" \
  -H "Authorization: Bearer $mem_tok" -F "file=@$CSV;type=text/csv")
[ "$code" = "403" ] || fail "member should get 403, got $code"

say "no JWT → 401"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$AUTH/import-users" -F "file=@$CSV;type=text/csv")
[ "$code" = "401" ] || fail "no-jwt got $code"

say "bad header CSV → 400"
BAD_CSV="$TMP/bad.csv"
echo "name,email,pw,role" > "$BAD_CSV"; echo "a,a@b.c,p,member" >> "$BAD_CSV"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$AUTH/import-users" \
  -H "Authorization: Bearer $mgr_tok" -F "file=@$BAD_CSV;type=text/csv")
[ "$code" = "400" ] || fail "bad header got $code"

say "all-ok CSV → 200"
GOOD="$TMP/good.csv"
cat > "$GOOD" <<EOF
username,email,password,role
g1,g1+$STAMP@example.com,password123,member
g2,g2+$STAMP@example.com,password123,member
EOF
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$AUTH/import-users" \
  -H "Authorization: Bearer $mgr_tok" -F "file=@$GOOD;type=text/csv")
[ "$code" = "200" ] || fail "all-ok got $code"

printf "\033[32m✓ phase-04 e2e PASS\033[0m\n"
