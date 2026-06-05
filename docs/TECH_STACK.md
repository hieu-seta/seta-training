# Tech Stack — Why These Picks, What I'd Change

This project builds three Go microservices (auth, team, asset) plus an audit worker, glued by NATS JetStream + Postgres + Redis, observed via Loki/Grafana, all in docker-compose. The brief was deliberately open: pick the stack, defend the trade-offs. The picks below favour boring tech, single-process containers, and the smallest viable infra footprint that still demonstrates each requirement from the three stages.

**Language: Go.** Forced by the course. Comfortable choice anyway — strong stdlib, race detector, single static binary per service. Fits distroless deployment cleanly.

**HTTP framework: Gin.** Considered chi and stdlib `net/http`. Gin trades a little reflection overhead for batteries-included middleware (JWT, logger, recovery). At three services the saved scaffolding more than pays for the runtime cost. If we needed sub-µs latency I'd reach for chi; for an intern capstone Gin is the right ergonomics-vs-performance trade.

**API style: REST + JSON.** gRPC would beat REST on inter-service serialisation, but it doubles the tooling cost (proto pipeline, grpcurl, browser gateway) for three callers. REST is curlable, documentable in plain Markdown, and the e2e bash scripts in this repo prove it. OpenAPI yaml is a separate artifact when needed.

**Database: PostgreSQL 16, schema-per-service.** Considered Mongo, considered one Postgres instance per service. Domain is relational (users ↔ teams ↔ folders ↔ notes ↔ shares ↔ ACL) so Mongo would re-implement joins app-side. Schema-per-service keeps logical isolation without paying the cost of N DBs. The cost: one shared backup target and the temptation to cross-query — neither has hurt yet.

**ORM: GORM.** Chose velocity over the type-safety win sqlc would have given. The cost is real (runtime errors, easy N+1) so the integration suite hits a real Postgres via testcontainers-go — every repo method has a tagged `//go:build integration` test against tc-go PG, which catches both regressions and ORM quirks. sqlc would be safer on a real team; here, CI integration coverage compensates.

**Auth: JWT HS256, refresh family rotation.** Stateless access tokens (15 min TTL) + Redis-stored refresh tokens. Each refresh family carries an opaque id; reusing a rotated refresh wipes the family and forces re-login — a cheap theft-detection signal. Considered RS256, but for a single trust boundary HS256 with a shared secret is simpler. The `pkg/jwtauth` middleware rejects `none` and non-HS256 algorithms explicitly (RFC 8725 § 3.1) and is unit-tested for both.

**Message broker: NATS JetStream.** The default answer is Kafka. Kafka is heavier (KRaft or ZK, JVM, more ops) than this project warrants. NATS JetStream gives me durable streams, replay, at-least-once delivery, and 5-minute publish-side dedup via `Nats-Msg-Id` — at maybe 5 % of Kafka's operational cost. Two streams (`ACTIVITY`, `ASSETS`) carry events from team-svc and asset-svc; the audit worker reads both via durable consumers with `MaxDeliver=5`. Publisher errors don't roll back business transactions (publish-after-commit) so eventual consistency is the contract.

**Cache: Redis 7.** Two hot paths get cached. team-svc caches the per-team `Detail` payload behind a singleflight read-through (5 min TTL) and an ephemeral NATS consumer Dels on every `team.activity.*` event. asset-svc wraps the `teamclient.ManagersOf` call in a `CachedTeamClient` — that call fires on every ACL fall-through and was the obvious bottleneck. Cache failure is non-fatal: the wrapper falls back to a `cache.Noop` if Redis is unreachable.

**Concurrency: errgroup + bounded channels.** The CSV bulk import endpoint runs ten workers consuming a buffered `chan row` (cap 100). Partial failures are collected via a separate `chan RowError` and returned as HTTP 207 with `{processed, failed[]}`. Idem on email uniqueness is enforced at the DB layer; row races for the same email are non-deterministic but both losing-rows show up as `Failed`.

**Observability: zerolog → Promtail → Loki → Grafana.** Considered ELK; Loki wins on memory cost and slots cleanly into docker-compose. Every service emits JSON to stdout with a `req_id` field; a small Gin middleware reads or mints `X-Request-Id` and propagates it. Promtail auto-discovers containers via the Docker socket and labels by `container` and `service`. Grafana auto-loads the Loki datasource via `/deploy/grafana/provisioning`.

**Deployment: docker-compose monorepo.** No Kubernetes. Eleven containers, one network, named volumes, a one-shot migrator container per schema. `make up` brings the entire stack live. Each service is built into a distroless static image (≈ 20 MB).

**Repo shape: Go workspace.** Six modules in one workspace — `pkg` for shared libraries (jwtauth, events, cache, authclient, teamclient, httpx, logger, pg), four services, and the migrator wrapper. CI runs lint, unit, integration, and e2e jobs per module.

The biggest deferred call is the ACL CQRS read model documented in `docs/FUTURE.md` — same NATS streams, one extra consumer, denormalised permission lookups, O(1) reads. The infra to do it already exists.
