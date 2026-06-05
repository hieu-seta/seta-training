# SOLUTION — Stack & Trade-offs

3 stages, solo dev, intern scope. Picks lean on tools I can run + debug in 1 day. Trade-offs called out per row, not hidden.

## 1. Stack

| Layer | Pick | Considered | Trade-off |
|---|---|---|---|
| Lang | Go 1.22 | — | Required by course |
| API | REST + JSON | gRPC, GraphQL | No schema contract → add OpenAPI yaml. Gain: curl works, no codegen step, browser dev tools just work |
| HTTP fw | Gin | chi, fiber, stdlib | Reflection overhead, opinionated context. Gain: middleware (jwt/cors/logger) already there, less scaffold code |
| DB | PostgreSQL 16, schema-per-service | Mongo, separate DB per svc | One box = weaker isolation. Gain: 1 backup target, real joins for ACL, JSONB if shape changes. Domain is relational — Mongo would re-implement joins app-side |
| ORM | GORM | sqlc, ent, raw sql | Runtime errors, easy to write N+1. Gain: CRUD in minutes, hooks for audit, auto-migrate for dev. sqlc safer but slower to iterate |
| Auth | JWT HS256 | sessions, OAuth, RS256 | Revocation lag → mitigate w/ 15m access + refresh token, refresh stored in Redis (revoke = delete key) |
| Broker | NATS JetStream | Kafka, RabbitMQ | Smaller ecosystem, lower ceiling on throughput. Gain: single Go binary, no ZK/KRaft, no Erlang, durable + replay + at-least-once already there |
| Cache | Redis 7 | Memcached, in-process | Extra network hop, another thing to run. Gain: TTL, pub/sub if needed, everyone on team has touched it |
| Logs | zerolog → stdout → Promtail → Loki → Grafana | ELK | Loki only indexes labels, not field text. Gain: way cheaper memory, fits container log model |
| Deploy | docker-compose | k8s, nomad | No HA, no rolling deploy. Fits stage-3 req of "one command brings everything up" |
| Repo | Monorepo, `go.work` | polyrepo | Couples release cadence. Solo dev → coupling is free. Shared `pkg/` w/o publishing modules |
| Migrations | golang-migrate | atlas, GORM auto-migrate in prod | Write SQL by hand. Gain: deterministic, replayable, no schema drift surprise |
| Config | envconfig + `.env` | viper, consul | No hot reload. 12-factor, ~20 LOC, done |
| Test | testify + testcontainers-go | mocks-only | Slower CI (~30s spin-up). Catches real integration bugs in PG/Redis/NATS |

## 2. Service Topology

```
            ┌─────────┐
client ───▶ │  Gin    │ each svc :8081/2/3
            └────┬────┘
   JWT (HS256, validated locally in middleware)
                 │
   ┌─────────────┼──────────────┬───────────────┐
   ▼             ▼              ▼               ▼
 auth-svc    team-svc       asset-svc      audit-worker
   │           │  │             │  │            ▲
   └──Postgres─┴──┴──Redis──────┴──┴──NATS──────┘
         (schema per svc)         (events)
```

- **auth-svc** — users, login, JWT mint, `POST /import-users` (worker pool)
- **team-svc** — teams, membership, emits `team.activity`
- **asset-svc** — folders, notes, ACL, emits `asset.changes`
- **audit-worker** — consumes both topics → `audit` table (append-only)

Inter-svc traffic kept minimal. team-svc + asset-svc trust JWT claims (`uid`, `role`); only hit auth-svc REST `/users/:id/exists` when strict check needed. No mesh, no discovery — docker DNS resolves it.

## 3. Stage Mapping

| Stage | Deliverable | New deps |
|---|---|---|
| 1 | auth + team REST, JWT, bcrypt | Gin, GORM, Postgres |
| 2 | asset CRUD + sharing + RBAC; CSV import w/ worker pool (10 workers, buffered chan, errgroup) | stdlib concurrency only |
| 3 | NATS events, Redis cache (write-through on events, 5m TTL safety), Loki, docker-compose | NATS, Redis, Loki/Promtail/Grafana |

## 4. Trade-offs I'd Get Asked About

1. **Single Postgres vs DB-per-service** — purist would split. Solo dev + intern scope wins. Schema-per-svc keeps logical line; split later = new DSN per svc, no code change.
2. **JWT vs session** — went stateless. Revocation gap real. Short TTL + Redis denylist for refresh covers logout/role-change.
3. **REST vs gRPC** — gRPC stronger for inter-svc, but doubles tooling (proto, grpcurl, browser gateway). 3 services not worth it yet.
4. **NATS vs Kafka** — Kafka is the default answer. Heavy though (KRaft, JVM, infra cost). NATS JetStream covers durable + replay + at-least-once at ~5% ops cost. Publisher interface in `pkg/events` so swap is contained.
5. **GORM vs sqlc** — GORM trades type safety for speed. Integration tests against real PG catch most regressions, so risk is bounded.
6. **Cache invalidation** — TTL-only would drift. Did write-through driven by `asset.changes` / `team.activity` events to meet "real-time team cache" req. Event loss → stale cache → 5m TTL caps blast radius.
7. **Monorepo** — release coupling. Team of 1, so it's a non-issue.

## 5. Repo Layout

```
.
├── go.work
├── docker-compose.yml
├── .env.example
├── deploy/{loki,promtail,grafana,nats}/
├── migrations/{auth,team,asset}/
├── pkg/
│   ├── jwtauth/      # mint + middleware
│   ├── events/       # NATS publisher + subjects
│   ├── cache/        # redis wrapper
│   ├── httpx/        # error → status mapper
│   └── logger/       # zerolog
└── services/
    ├── auth/         # cmd/main.go, internal/{handler,service,repo,model}
    ├── team/
    ├── asset/
    └── audit-worker/
```

## 6. Future Bottleneck Pitch (demo)

Read path on ACL checks will go first. asset-svc hits PG on every read to confirm permission — fine at 100 rps, melts at 10k. Re-arch = **CQRS read model**: `audit-worker` already consumes `asset.changes`, extend it to project ACL into a denormalized Redis structure (`perm:{userId}` → set of asset ids). Read path becomes O(1) Redis lookup; write path keeps PG. No new infra, same NATS stream — just one more consumer.

## 7. Why This Shape

Optimizing for: few binaries to install, few concepts to explain, stdlib where possible. No k8s, no service mesh, no schema registry, no codegen. External infra = Postgres + Redis + NATS + Loki/Promtail/Grafana, all single-container.
