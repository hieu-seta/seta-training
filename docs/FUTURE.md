# Future Bottleneck Pitch — CQRS ACL Read Model

## The Bottleneck

ACL checks dominate the asset-svc read path. Every `GET /folders/:id` or `GET /notes/:id` runs the three-source resolver:

1. owner check (one PG lookup on the asset row)
2. explicit share lookup (note shares ∪ parent folder shares — one SQL UNION)
3. manager-of-owner fallback (one HTTP call to team-svc, which runs its own JOIN)

At 100 RPS that's already three trips per read. At 10,000 RPS the team-svc JOIN and the inter-service HTTP are the obvious losers. The Redis `ManagersOf` cache (phase-07) buys headroom by collapsing step 3 into a Redis GET, but step 1 + step 2 still hit Postgres on every request.

## The Move: CQRS Read Model in audit-worker

The infrastructure to project events into a read-optimised store **already exists**. The audit worker already consumes every state change in both streams and writes them to `audit.events`. Extending it to maintain a second projection costs one more `Insert` per event.

### The projection

For every `(uid, asset_id, op)` combination, store the decision in a denormalised Redis hash:

```
perm:{uid}              = HASH
  field=<asset_id>      val="rw" | "ro" | (absent → deny)
```

Or a bitset variant per asset for higher density at scale. The audit worker recomputes the affected entries from each incoming event:

| Event                                       | Keys to update                                              |
|---------------------------------------------|-------------------------------------------------------------|
| `asset.changes.folder_created`              | `perm:{owner}` ← "rw"                                       |
| `asset.changes.folder_shared`               | `perm:{target}` ← share access                              |
| `asset.changes.folder_revoked`              | `perm:{target}` ← delete entry (or "deny")                  |
| `asset.changes.note_created` (inside fldr)  | `perm:{owner}` and copy folder shares to note               |
| `team.activity.member_added` to mgr's team  | `perm:{mgr}` ← "ro" for every asset owned by every member   |
| `team.activity.manager_added`               | similar, for the new manager                                |

### The read path becomes O(1)

```go
// Old (three round-trips, two stores)
err := acl.Check(ctx, uid, kind, assetID, op)

// New (one Redis HGET)
v, err := rdb.HGet(ctx, "perm:"+uid, assetID).Result()
if err == redis.Nil { return ErrForbidden }
allowed := strings.Contains(v, opLetter(op))
```

Latency drops from ~5 ms (PG + HTTP) to <1 ms (Redis). Throughput rises with Redis instance count.

## Migration Plan

| Step | What                                                                                          | Risk |
|------|-----------------------------------------------------------------------------------------------|------|
| 1    | Add a second projection to `audit-worker` (feature-flagged, no read path yet)                 | low — write-only, isolated |
| 2    | Replay the historical streams to backfill `perm:*` (NATS replay from `DeliverAllPolicy`)      | low — idempotent inserts |
| 3    | Add a shadow read in `acl.Check`: do both old + new, compare, log mismatches                  | medium — must not break live reads |
| 4    | After N days of zero mismatches, flip a flag to read from Redis only                          | medium — feature-flag controlled |
| 5    | Delete the old SQL ACL UNION query + `ManagersOf` HTTP call                                   | low — dead code removal |

## Why It Works

- **Same infrastructure.** No new broker, no new database, no new deploy target.
- **Eventually consistent.** Events already drive everything; the same 5-minute dedup window guarantees idempotent projection.
- **Reversible.** Until step 4, the old code path stays alive.
- **Observable.** The shadow-read mismatch metric ships through the same Loki pipeline as everything else.

## What I'd Watch

- **Projection lag.** Add a metric: time between event publish and Redis key update. Alert if > 5 s.
- **Cache eviction.** `perm:*` keys can be large per uid. Either accept eviction (and fall back to the old path on miss) or scale Redis vertically.
- **Manager bulk updates.** Adding a manager to a large team re-projects N entries. Batch via `HMSET`, throttle if needed.

## What I'd NOT Build Yet

- **A full Read DB (Postgres replica).** Redis is enough at the scales any junior project ever hits.
- **Per-asset bitsets.** Optimisation for the optimised path. Not worth it before profiling.
- **gRPC for inter-service ACL queries.** Becomes irrelevant — there are no inter-service queries left after this move.
