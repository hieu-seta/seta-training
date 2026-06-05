# Demo Script — 3 Minutes

> Goal: end-to-end live walkthrough of every stage requirement in under 3 minutes. Pair with terminal + Grafana on a second window.

## 0:00 – 0:30  Boot + intro
```
make up
```
While compose comes up, name the moving parts:
> "Three Go services (auth, team, asset) + audit worker. Postgres schema per service, NATS JetStream for events, Redis for caches, Loki/Grafana for logs. One compose file, one command."

## 0:30 – 1:00  Stage 1 — Identity + Teams
```
bash scripts/e2e/01-auth.sh
bash scripts/e2e/02-team.sh
```
> "JWT HS256 with refresh-token family rotation — replaying an old refresh wipes the family and forces re-login. Team-svc enforces manager-only writes and a main-manager rule for promotions."

## 1:00 – 1:30  Stage 2 — Assets + Concurrency
```
bash scripts/e2e/03-asset.sh
bash scripts/e2e/04-import.sh
```
> "Three-source ACL: owner / explicit share / manager-of-owner. Sharing a folder grants read on every note inside. The CSV importer runs ten workers via errgroup, returns 207 with the failed rows."

## 1:30 – 2:00  Stage 3 — Events + Audit + Cache
```
bash scripts/e2e/05-events.sh
bash scripts/e2e/06-audit.sh
bash scripts/e2e/07-cache.sh
```
> "Every mutation publishes to JetStream; the audit worker consumes both streams with dedup via Nats-Msg-Id. Team and asset services cache the hot paths and an event-driven invalidator Dels keys on every activity message."

## 2:00 – 2:30  Observability
```
bash scripts/e2e/08-observability.sh
open http://localhost:3001
```
> "JSON logs flow Promtail → Loki. Every request carries an `X-Request-Id` that traces through every service. Grafana datasource is auto-provisioned."

In Grafana, run:
```
{container=~"seta-(auth|team|asset)"} | json | req_id != ""
```

## 2:30 – 3:00  Future bottleneck pitch
Pull up `docs/FUTURE.md` (one paragraph). The pitch:
> "The hottest read path is the ACL check — every asset GET potentially calls `ManagersOf` on team-svc. The cache helps but it's still a fetch on miss. The next move is a CQRS read model: a new consumer projects every relevant event into a denormalised Redis hash keyed by `(uid, asset)`. Asset reads become O(1) Redis lookups, writes stay in Postgres, no new infra — same NATS stream, one extra consumer."

## Wrap
```
docker compose down -v
```
