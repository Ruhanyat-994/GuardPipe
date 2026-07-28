# ADR-0009 — goose for database migrations, and no ORM

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M1 |
| Supersedes | — |

## Context

Two related decisions about the data access layer:

1. How database schema changes are managed, given that **six developers share one schema over three weeks** — identified in [03 §12](../03-architecture-overview.md#12-risks-and-technical-debt) as a top risk.
2. How Go code talks to PostgreSQL.

Requirements:
- Version-controlled, reviewable schema changes.
- Migrations applied automatically at startup so no developer's environment drifts.
- Collisions between concurrent developers must surface at **merge time**, not at runtime.
- Query performance matters on the findings path ([06 §8](../06-database-design.md#8-index-summary-and-justification)).
- The team must be able to read exactly what SQL hits the database.

## Options considered

### Migrations
**A — goose:** plain `.sql` files with `-- +goose Up/Down`, embeddable, Go library or CLI.
**B — golang-migrate:** separate `.up.sql`/`.down.sql` files.
**C — Atlas:** declarative schema with automatic diffing.
**D — ORM auto-migration (GORM `AutoMigrate`).**

### Query layer
**E — sqlc:** generates typed Go from hand-written SQL.
**F — GORM:** full ORM.
**G — sqlx:** thin `database/sql` helper.
**H — Raw `pgx`.**

## Decision

**goose** for migrations, **sqlc** for the majority of queries, with hand-written `pgx` for dynamic filter queries. **No ORM.**

## Rationale

### Migrations

goose over golang-migrate on a small but real ergonomic point: one file per migration containing both directions keeps a change reviewable as a single unit, and `//go:embed` of the migrations directory means the binary carries its own schema — no volume mount, no separate migration container, no "did you run migrations?" in Compose.

Atlas is the more sophisticated tool and would be the right answer for a long-lived production schema. Its declarative diffing is exactly the wrong property here: with six people changing the schema concurrently, we *want* explicit, ordered, numbered files that **collide in git** when two people change things at once. A merge conflict on `00013_*.sql` is a five-minute conversation; two silently-merged declarative changes producing an unexpected diff is an afternoon.

GORM's `AutoMigrate` was rejected outright. It cannot express the schema we need (partial indexes, GIN indexes, generated `tsvector` columns, `ON DELETE RESTRICT`, native enums), it does not handle destructive changes, and it gives no reviewable artifact. For a project whose deliverable includes a documented database design, an invisible schema is disqualifying.

### Query layer

sqlc's proposition is unusual and exactly right for this team: **you write SQL, it generates the Go types.** Queries are checked against the real schema at generation time, so a column rename breaks the build rather than production. Everyone can read the SQL — important when four of six developers are still learning Go.

GORM was rejected for reasons that go beyond taste:
- Generated SQL is unpredictable, and the findings query with six optional predicates is a hot path.
- Its N+1 behaviour is easy to trigger accidentally and hard to notice.
- It obscures what actually executes, which is a poor property in a security product.
- Learning GORM's semantics is not obviously cheaper than learning SQL, and SQL knowledge transfers.

Dynamic filter queries — the findings list with any combination of severity, engine, status, CWE, path, and search — do not fit sqlc's static model. Those use a small parameterised query builder over `pgx`. **String concatenation into SQL is forbidden**, in a project that ships a rule detecting exactly that (`codescan.injection.sql-string-concat`). Building our own SQL injection would be the single most embarrassing possible defect.

### The change protocol

The tooling only works alongside process. [06 §12](../06-database-design.md#12-schema-change-protocol) requires: announce first, schema-only PR, two approvals, claim your number early, renumber on rebase, never edit a merged migration. The tool makes collisions *visible*; the protocol makes them *cheap*.

## Consequences

### Positive
- Schema changes are plain SQL — reviewable by anyone, in the PR diff.
- Migrations embedded in the binary; no separate step or container.
- Sequential numbering forces concurrent changes to conflict at merge time, where it is cheap.
- sqlc gives compile-time-checked queries with zero runtime reflection.
- Everyone can read exactly what SQL runs.
- Full access to PostgreSQL features — partial indexes, GIN, generated columns, enums.

### Negative
- **More code to write than an ORM.** Accepted; predictability is worth it.
- `make sqlc` must be re-run after every query change — a forgotten regeneration causes a confusing build error.
- Migration number collisions will happen. That is the design; the protocol handles them.
- Dynamic queries need a hand-written builder, which must be reviewed carefully for injection safety.
- No automatic schema-drift detection; the documentation must be kept in sync by discipline (enforced by CODEOWNERS on `06-database-design.md`).

### Neutral
- Rollback (`goose down`) exists and is tested, but production practice is forward-fix.

## Revisit when

- The schema stabilises post-1.0 and declarative management (Atlas) becomes attractive.
- Hand-written queries become a measured productivity bottleneck.
