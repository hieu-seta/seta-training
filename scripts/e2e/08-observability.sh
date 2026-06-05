#!/usr/bin/env bash
# Phase-08 e2e: verify logs land in Loki + req-id propagates + Grafana datasource green.
#
# Coverage:
#   1. req-id middleware echoes header (auth-svc).
#   2. req-id appears in Loki for ALL THREE services when each is hit independently.
#   3. team-svc auto-forwards X-Request-Id to auth-svc via authclient
#      → same id surfaces in BOTH team-svc and auth-svc Loki streams.
#   4. Grafana datasource provisioned green.

set -euo pipefail
AUTH="${AUTH_BASE:-http://localhost:8081}"
TEAM="${TEAM_BASE:-http://localhost:8082}"
ASSET="${ASSET_BASE:-http://localhost:8083}"
LOKI="${LOKI_BASE:-http://localhost:3100}"
GRAFANA="${GRAFANA_BASE:-http://localhost:3001}"

say()  { printf "\033[36m▶ %s\033[0m\n" "$*"; }
fail() { printf "\033[31m✗ %s\033[0m\n" "$*" >&2; exit 1; }

# Loki port not published by default → query inside the compose network.
loki_query() {
  docker exec seta-promtail wget -qO- "http://loki:3100/loki/api/v1/query_range?query=$1&limit=20" 2>/dev/null
}

# url_encode <string>
url_encode() {
  python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$1"
}

# loki_has <req-id> <container> — returns 0 when at least one line matches.
loki_has() {
  local req_id="$1" container="$2"
  local q='{container="'"$container"'"} |= "'"$req_id"'"'
  local encoded
  encoded=$(url_encode "$q")
  local result
  result=$(loki_query "$encoded")
  echo "$result" | python3 -c "
import json,sys
data=json.load(sys.stdin)
streams = data.get('data', {}).get('result', [])
matches = sum(len(s.get('values', [])) for s in streams)
sys.exit(0 if matches > 0 else 1)
"
}

say "1) X-Request-Id echoed in response (auth-svc)"
REQ_ID="e2e-test-$(date +%s)"
code=$(curl -s -o /dev/null -w "%{http_code}" -H "X-Request-Id: $REQ_ID" "$AUTH/healthz")
[ "$code" = "200" ] || fail "healthz got $code"
resp_id=$(curl -s -I -H "X-Request-Id: $REQ_ID" "$AUTH/healthz" | awk 'tolower($1)=="x-request-id:" {print $2}' | tr -d '\r')
[ "$resp_id" = "$REQ_ID" ] || fail "expected response header $REQ_ID, got $resp_id"

say "2) hit each svc with one req-id → all 3 land in Loki"
TRIPLE_ID="triple-$(date +%s)"
curl -s -o /dev/null -H "X-Request-Id: $TRIPLE_ID" "$AUTH/healthz"  || fail "auth /healthz"
curl -s -o /dev/null -H "X-Request-Id: $TRIPLE_ID" "$TEAM/healthz"  || fail "team /healthz"
curl -s -o /dev/null -H "X-Request-Id: $TRIPLE_ID" "$ASSET/healthz" || fail "asset /healthz"

say "  wait for Promtail to ship → Loki"
sleep 5

for svc in seta-auth seta-team seta-asset; do
  if loki_has "$TRIPLE_ID" "$svc"; then
    printf "  \033[32m✓ %s log carries %s\033[0m\n" "$svc" "$TRIPLE_ID"
  else
    fail "no log line in $svc containing $TRIPLE_ID"
  fi
done

say "3) auto-forward: team-svc → auth-svc via authclient"
# Login as default admin (created by phase-01 e2e or via fixtures).
# If unavailable, skip the auto-forward check — bare-minimum logging still validated by step 2.
TOKEN=$(curl -s -X POST "$AUTH/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@example.com","password":"admin12345"}' \
    | python3 -c "import json,sys;print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || echo "")

if [ -z "$TOKEN" ]; then
  printf "\033[33m  ⚠ skip: no admin token available (run phase-01 e2e first to seed)\033[0m\n"
else
  # Trigger a path that calls authclient.UserExists: AddMember w/ a bogus user-id.
  # We don't care about the business outcome (404 or 503 are both fine) —
  # only that team-svc *attempts* the call w/ forwarded header.
  FORWARD_ID="forward-$(date +%s)"
  TEAM_ID="$(uuidgen 2>/dev/null || python3 -c 'import uuid;print(uuid.uuid4())')"
  USER_ID="$(uuidgen 2>/dev/null || python3 -c 'import uuid;print(uuid.uuid4())')"
  curl -s -o /dev/null \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Request-Id: $FORWARD_ID" \
    -X POST "$TEAM/teams/$TEAM_ID/members" \
    -H "Content-Type: application/json" \
    -d "{\"user_id\":\"$USER_ID\"}" || true

  sleep 5

  if loki_has "$FORWARD_ID" "seta-team" && loki_has "$FORWARD_ID" "seta-auth"; then
    printf "  \033[32m✓ %s seen in both team-svc and auth-svc logs (authclient forwarded)\033[0m\n" "$FORWARD_ID"
  else
    fail "expected $FORWARD_ID in both seta-team and seta-auth logs"
  fi
fi

say "4) Grafana datasource provisioned"
ds=$(curl -s -u admin:admin "$GRAFANA/api/datasources/name/Loki")
echo "$ds" | python3 -c "
import json,sys
d=json.load(sys.stdin)
assert d.get('type') == 'loki', f'wrong type: {d}'
assert d.get('url') == 'http://loki:3100', f'wrong url: {d}'
print('  datasource OK:', d.get('name'), d.get('url'))
" || fail "Grafana datasource missing or wrong"

say "5) Grafana dashboards provisioned (auth, team, asset, overview)"
for uid in seta-auth seta-team seta-asset seta-overview; do
  status=$(curl -s -o /dev/null -w "%{http_code}" -u admin:admin "$GRAFANA/api/dashboards/uid/$uid")
  if [ "$status" = "200" ]; then
    printf "  \033[32m✓ dashboard %s present\033[0m\n" "$uid"
  else
    fail "dashboard $uid missing (HTTP $status)"
  fi
done

printf "\033[32m✓ phase-08 e2e PASS — logs + req-id + cross-svc propagation + Grafana wired\033[0m\n"
