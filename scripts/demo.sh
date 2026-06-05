#!/usr/bin/env bash
# 3-minute live demo. Run: bash scripts/demo.sh
# Brings the stack up, runs every e2e script, opens Grafana.

set -euo pipefail

cd "$(dirname "$0")/.."

step() { printf "\n\033[1;36m▶▶▶ %s\033[0m\n" "$*"; }
pause() { printf "\033[2m(press enter)\033[0m"; read -r; }

step "1. Boot the whole stack"
docker compose up -d
echo "  waiting for svc healthchecks…"
for i in $(seq 1 30); do
  curl -fsS http://localhost:8081/healthz >/dev/null 2>&1 && \
  curl -fsS http://localhost:8082/healthz >/dev/null 2>&1 && \
  curl -fsS http://localhost:8083/healthz >/dev/null 2>&1 && break
  sleep 2
done

step "2. Run the full e2e suite (auth → team → asset → import → events → audit → cache → observability)"
bash scripts/e2e/run_all.sh

step "3. Open Grafana (admin/admin, Loki datasource auto-provisioned)"
URL="http://localhost:${GRAFANA_PORT:-3001}"
echo "  $URL"
if command -v open >/dev/null; then
  open "$URL" || true
fi

step "Done. Stack stays up. Stop w/: docker compose down -v"
