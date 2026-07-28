# ADR-0003 — PostgreSQL as system of record, Redis for queue and cache

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M1, full team |
| Supersedes | — |

## Context

GuardPipe stores structured, highly relational data — organisations, users, projects, scans, jobs, findings, evidence, dependencies, audit records — with heavy filtered querying over findings. It also needs a job queue for asynchronous scan execution and a cache for advisory lookups and AI responses, both of which are essential to staying inside the Gemini and OSV rate limits.

Requirements:
- Relational integrity with cascading deletes (a deleted project must not orphan 200k findings).
- Rich filtering: severity, engine, status, CWE, CVE, file path prefix, full-text search.
- Arrays (CWE/CVE lists) and semi-structured data (the `location` union, evidence payloads).
- A job queue with at-least-once delivery.
- Caching with TTL.
- Runs in Docker Compose with zero configuration.

## Options considered

### Option A — PostgreSQL + Redis
Relational store plus a dedicated cache/queue.

### Option B — PostgreSQL only
Use `SKIP LOCKED` for the queue and a table for the cache.

### Option C — MongoDB
Document store for findings, which are semi-structured.

### Option D — PostgreSQL + a message broker (RabbitMQ/NATS)

## Decision

**Option A — PostgreSQL 16 as the system of record, Redis 7 for the job queue, cache, rate limiting, and live progress.**

Redis holds **no persistent state** and is treated as losable (NFR-REL-003).

## Rationale

PostgreSQL is not a close call. The data is relational, the queries are relational, and PostgreSQL uniquely gives us native arrays (`TEXT[]` for CWE/CVE), `JSONB` with functional indexes (the `location` union), generated `tsvector` columns for full-text search, native enums, and partial indexes — all of which the findings query path uses directly ([06 §8](../06-database-design.md#8-index-summary-and-justification)).

MongoDB was rejected because the *appeal* is illusory. Findings look semi-structured, but the queries are joins and aggregates across scans, jobs, projects, and rules. Denormalising that into documents would mean either duplicating data or performing joins in application code. Referential integrity — a real requirement, given cascading deletes across five tables — would move into our code, where it would eventually be wrong.

Option B (Postgres-only queue via `SELECT ... FOR UPDATE SKIP LOCKED`) genuinely works and is a legitimate simplification. We chose against it for three reasons: the advisory and AI caches need TTL semantics that Redis provides natively and Postgres does not; live scan progress is polled every 2 seconds by every open browser tab, and that must not touch the primary database; and the token-bucket rate limiter is trivial in Redis and awkward in SQL. Redis is one Compose line and no operational burden at this scale.

Option D was rejected as over-engineering. A Redis list with `BRPOPLPUSH` plus a reaper for orphaned claims gives at-least-once delivery, which — combined with fingerprint-based idempotent inserts (NFR-REL-002) — is sufficient. A broker adds a component, an operational concept, and a failure mode for a delivery guarantee we do not need.

The **losable-Redis** rule is the important design constraint that follows. Job state of record lives in `scan_jobs`; a startup sweep re-enqueues anything `queued` or `running`. This means Redis can be flushed at any time — including by a developer running `FLUSHALL` in week two — without data loss.

## Consequences

### Positive
- Referential integrity enforced by the database, including cascades.
- Complex filtered queries stay in SQL where they belong.
- `JSONB` handles the `location` union without 18 nullable columns.
- Full-text search with no additional service.
- Redis makes caching, rate limiting, and progress trivial.
- Both are standard Docker images with no configuration.
- Straightforward migration path to managed services later.

### Negative
- **Two data services instead of one.** More to start, more to understand. Accepted — mitigated by Redis being stateless from our perspective.
- **A shared schema across six developers is a coordination cost.** Mitigated by the protocol in [06 §12](../06-database-design.md#12-schema-change-protocol).
- **Redis adds a partial failure mode.** Mitigated: if Redis is down the system degrades (jobs run synchronously, caches miss) rather than failing.
- Migrations become a serialisation point between developers.

### Neutral
- `findings` will eventually want partitioning by date. Documented as the growth path, deliberately not implemented — at 200k rows it would be speculative complexity.

## Revisit when

- `findings` exceeds a few million rows and query performance degrades → partition by `created_at`.
- Multi-tenancy becomes real → PostgreSQL row-level security.
- Job volume justifies a real broker → the `Queue` interface makes it a swap.
