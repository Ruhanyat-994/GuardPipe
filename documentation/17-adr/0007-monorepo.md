# ADR-0007 — Single monorepo

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M1 |
| Supersedes | — |

## Context

The project comprises a Go backend, a React frontend, deployment configuration, bash pentest scripts, test fixtures, and documentation. Six people work on all of it simultaneously over four weeks.

## Options considered

### Option A — Single monorepo
Everything in `github.com/Ruhanyat-994/GuardPipe`.

### Option B — Separate backend and frontend repositories

### Option C — Repository per component
Backend, frontend, scripts, infrastructure, documentation.

## Decision

**Option A — one repository.**

```
/                      go.mod, Makefile, README
/cmd, /internal        Go backend
/frontend              React SPA
/deploy                Compose, Dockerfiles
/documentation         this folder
/testdata/fixtures     golden fixture repositories
/.github               workflows, CODEOWNERS, templates
```

## Rationale

This is not a difficult decision at this scale, but it is worth recording because "backend repo and frontend repo" is a common default that would actively hurt here.

An API change and its frontend consumption belong in **one atomic commit**. In split repositories, a contract change requires two PRs, in a specific order, with a window in which `main` on one repository is broken against `main` on the other. Over three weeks with a contract still stabilising, that window would open dozens of times.

A single repository also gives one CI pipeline that can test backend and frontend against each other, one issue tracker, one board, one CODEOWNERS file governing everything including documentation, and one version tag that means something. A `v0.3.0` tag in a monorepo describes a system; in three repositories it describes a fragment.

The usual arguments against monorepos — build times, tooling complexity, unrelated CI runs — apply at a scale far beyond this project. Path filters keep CI targeted; the whole pipeline runs in under five minutes regardless.

Test fixtures deserve a specific note: the golden fixture repositories ([15 §5](../15-testing-strategy.md#5-the-golden-fixture-repositories)) are consumed by backend tests and referenced in demo seeding. Keeping them in-repo means a fixture change and the rule change it supports land together.

## Consequences

### Positive
- Atomic cross-cutting changes: API + frontend + documentation in one commit, one review.
- One CI pipeline, one board, one set of labels, one CODEOWNERS.
- Documentation is versioned alongside the code it describes — enabling the same-PR sync rule ([14 §10](../14-github-workflow.md#10-documentation-workflow)).
- New contributors clone once and have everything.
- Version tags describe the whole system.

### Negative
- Frontend developers see backend commits and vice versa — noise, mitigated by conventional-commit scopes.
- CI must use path filters to avoid running Go tests on a CSS change.
- The repository grows faster; fixtures add size.
- Access control is all-or-nothing — irrelevant for a six-person team.

### Neutral
- `go.mod` at the root with `frontend/` excluded from the Go module is standard and unremarkable.

## Revisit when

- Components need genuinely independent release cadences.
- The repository grows large enough that clone or CI time becomes a real friction.
- External contributors need access to one component but not others.
