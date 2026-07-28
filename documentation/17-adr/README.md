# 17 — Architecture Decision Records

| Field | Value |
|---|---|
| **Document** | ADR Index |
| **Project** | GuardPipe |
| **Format** | MADR / Nygard |
| **Last updated** | 2026-07-29 |

---

## What an ADR is

An Architecture Decision Record captures **one significant decision**: the context that forced it, the options considered, what was chosen, and what that costs.

ADRs exist because in week three someone will ask "why aren't we using microservices?" or "why not GORM?" — and the answer should be a link, not a re-argument. A decision that was debated once and never recorded gets debated again.

## Rules

| Rule | Detail |
|---|---|
| **Immutable** | An accepted ADR is never edited. Changed your mind? Write a new one that supersedes it |
| **Numbered sequentially** | `NNNN-kebab-title.md` |
| **One decision each** | Not "Sprint 1 decisions" |
| **Honest about consequences** | The negative consequences section is the most valuable part. An ADR with no downsides listed is not a decision, it is an advertisement |
| **Written when the decision is made** | Not retrofitted at the end |
| **Status is explicit** | `Proposed` · `Accepted` · `Superseded by ADR-NNNN` · `Deprecated` |

## When to write one

Write an ADR when the decision is **expensive to reverse** or **will be questioned later**:

- Choosing or rejecting a framework, library, or architectural pattern
- A data model or contract decision that many things will depend on
- A security trade-off, especially an accepted risk
- Anything the team argued about for more than 15 minutes
- Anything a reviewer would reasonably say "why did you do it that way?" about

Do not write one for routine implementation choices.

## Template

```markdown
# ADR-NNNN — <Title>

| Status | Accepted |
|---|---|
| Date | YYYY-MM-DD |
| Deciders | … |
| Supersedes | — |

## Context
What forces are at play? What constraint or problem requires a decision?

## Options considered
### Option A — …
### Option B — …
### Option C — …

## Decision
What we chose, stated plainly.

## Rationale
Why this option beat the others, given our specific constraints.

## Consequences
### Positive
### Negative
### Neutral

## Revisit when
The condition under which this decision should be reconsidered.
```

---

## Index

| # | Title | Status | Date | Affects |
|---|---|---|---|---|
| [0001](0001-modular-monolith.md) | Modular monolith over microservices | Accepted | 2026-07-29 | Whole system |
| [0002](0002-go-and-gin.md) | Go with the Gin web framework | Accepted | 2026-07-29 | Backend |
| [0003](0003-postgresql-and-redis.md) | PostgreSQL as system of record, Redis for queue and cache | Accepted | 2026-07-29 | Data layer |
| [0004](0004-gemini-llm-provider.md) | Gemini as the LLM, behind a provider port | Accepted | 2026-07-29 | AI features |
| [0005](0005-sandboxed-scan-execution.md) | In-process Go analysis with Docker-sandboxed shell execution | Accepted | 2026-07-29 | Engines, security |
| [0006](0006-react-vite-spa.md) | React + Vite SPA over Next.js | Accepted | 2026-07-29 | Frontend |
| [0007](0007-monorepo.md) | Single monorepo | Accepted | 2026-07-29 | Repository |
| [0008](0008-mermaid-diagrams.md) | Mermaid-in-Markdown for all diagrams | Accepted | 2026-07-29 | Documentation |
| [0009](0009-goose-migrations.md) | goose for database migrations, no ORM | Accepted | 2026-07-29 | Data layer |
| [0010](0010-own-scanners.md) | Build our own scanners rather than wrapping existing tools | Accepted | 2026-07-29 | Engines |
