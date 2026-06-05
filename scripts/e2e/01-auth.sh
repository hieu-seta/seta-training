#!/usr/bin/env bash
# Phase-01 e2e: hit auth-svc on :8081 (assumes `make up` running + migrations applied).
# Exits 1 on any failure. Uses curl + python3 (json parsing).

set -euo pipefail

BASE="${AUTH_BASE:-http://localhost:8081}"
PASS="password123"
EMAIL="e2e+$(date +%s)@example.com"

say() { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null || fail "missing tool: $1"; }
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

need curl
need python3

say "healthz"
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/healthz")
[ "$code" = "200" ] || fail "healthz got $code"

say "POST /users (create)"
resp=$(curl -s -X POST "$BASE/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"e2e\",\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"role\":\"manager\"}")
uid=$(echo "$resp" | json "['id']")
[ -n "$uid" ] || fail "no id in create resp: $resp"

say "POST /users duplicate → 409"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"e2e\",\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"role\":\"manager\"}")
[ "$code" = "409" ] || fail "expected 409, got $code"

say "POST /users bad email → 400"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/users" -H 'Content-Type: application/json' \
  -d "{\"username\":\"x\",\"email\":\"not-email\",\"password\":\"$PASS\",\"role\":\"member\"}")
[ "$code" = "400" ] || fail "expected 400 on bad email, got $code"

say "POST /login wrong pw → 401"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"WRONG\"}")
[ "$code" = "401" ] || fail "wrong pw got $code"

say "POST /login → tokens"
login=$(curl -s -X POST "$BASE/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")
access=$(echo "$login" | json "['access']")
refresh=$(echo "$login" | json "['refresh']")
[ -n "$access" ] && [ -n "$refresh" ] || fail "no tokens: $login"

say "GET /users (no JWT) → 401"
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/users")
[ "$code" = "401" ] || fail "expected 401, got $code"

say "GET /users (with JWT) → 200"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $access" "$BASE/users")
[ "$code" = "200" ] || fail "expected 200, got $code"

say "GET /users/\$uid/exists → 200"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $access" "$BASE/users/$uid/exists")
[ "$code" = "200" ] || fail "expected 200, got $code"

say "GET /users/<random>/exists → 404"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $access" "$BASE/users/00000000-0000-0000-0000-000000000000/exists")
[ "$code" = "404" ] || fail "expected 404, got $code"

say "POST /refresh → rotated"
newpair=$(curl -s -X POST "$BASE/refresh" -H 'Content-Type: application/json' -d "{\"refresh\":\"$refresh\"}")
newrefresh=$(echo "$newpair" | json "['refresh']")
[ "$newrefresh" != "$refresh" ] || fail "refresh not rotated"

say "POST /refresh (replay original) → 401 + family wiped"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/refresh" -H 'Content-Type: application/json' -d "{\"refresh\":\"$refresh\"}")
[ "$code" = "401" ] || fail "replay expected 401, got $code"

# After family wipe, even the rotated refresh is invalid.
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/refresh" -H 'Content-Type: application/json' -d "{\"refresh\":\"$newrefresh\"}")
[ "$code" = "401" ] || fail "post-wipe rotated refresh expected 401, got $code"

say "POST /logout (idempotent on invalid) → 204"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/logout" -H 'Content-Type: application/json' -d "{\"refresh\":\"$refresh\"}")
[ "$code" = "204" ] || fail "logout expected 204, got $code"

printf "\033[32m✓ phase-01 e2e PASS\033[0m\n"
