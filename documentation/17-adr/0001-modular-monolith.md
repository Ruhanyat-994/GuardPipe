# ADR-0001 — Modular monolith over microservices

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | Full team |
| Supersedes | — |

## Context

GuardPipe comprises nine functional modules (seven security engines plus AI and scoring) built by six people in four weeks. An earlier plan proposed four go-zero microservices communicating over gRPC with protobuf contracts, a Redis event bus, and one database schema per service.

The forces:

- **Hard deadline, 4 weeks, part-time contributors** with mixed Go experience.
- **No scaling requirement.** The system serves one organisation on one laptop.
- **No independent deployment requirement.** Everything ships at once, on demo day.
- **High module count** — nine modules is genuinely a lot of surface area, and the boundaries between them do need to be real.
- **Four people writing engines in parallel** need to not block each other.

## Options considered

### Option A — Microservices (go-zero + gRPC)
Four services: gateway, scan, ai-copilot, dashboard. Protobuf contracts, separate schemas, Redis event bus.

### Option B — Modular monolith
One Go binary. Strict internal module boundaries enforced by `internal/` packages, interface-only cross-module communication, and code review.

### Option C — Unstructured monolith
One binary, no enforced boundaries. Fastest to start.

## Decision

**Option B — modular monolith.** One deployable binary with hard internal boundaries, a shared PostgreSQL database using module-prefixed tables, and a documented dependency rule.

## Rationale

The question is not "which architecture is better in general" but "which architecture solves a problem we actually have."

| Benefit of microservices | Do we have this problem? |
|---|---|
| Independent scaling | **No.** One laptop, one organisation |
| Independent deployment | **No.** One release, one day |
| Team autonomy across many teams | **No.** Six people, one room |
| Technology heterogeneity | **No.** All Go |
| Fault isolation between services | Partially — solved instead by panic recovery per job ([04 §6.4](../04-backend-architecture.md#64-panic-containment)) |

Against zero realised benefits, microservices would cost: protobuf toolchain setup, service discovery, four Dockerfiles, distributed tracing to debug anything, network failure handling between our own components, cross-service transaction problems, and a local environment nobody can run comfortably. Conservatively 4–6 days — a quarter of the schedule — spent on infrastructure that solves nothing.

Option C was rejected for the opposite reason: with four people writing engines simultaneously, unenforced boundaries become entangled within two weeks, and the last week becomes untangling instead of finishing.

The modular monolith also preserves the option. Module interfaces are the natural extraction seam: if GuardPipe ever needed `scan-service` as a separate process, the boundary is already drawn and the work is mechanical.

## Consequences

### Positive
- First working feature in hours, not days.
- One stack trace for any bug, not distributed tracing across four services.
- No distributed transactions, no eventual consistency, no partial-failure semantics between our own components.
- Local development is `make up`.
- Single binary deployment; ~25 MB distroless image.
- Onboarding cost near zero for a Go-capable developer.

### Negative
- **No independent scaling.** The whole binary scales as one unit. Accepted — mitigated by the `GUARDPIPE_ROLE=api|worker` split, which allows separating the two workloads later without a code change.
- **Shared database schema across six developers.** This is the real cost, and it is significant. Mitigated by the change protocol in [06 §12](../06-database-design.md#12-schema-change-protocol), module-prefixed tables, and a two-approval rule.
- **Boundaries are enforced by discipline, not by process isolation.** A determined shortcut can violate them. Mitigated by `internal/` packages, an explicit dependency rule, and a PR checklist item.
- **A pathological engine could affect the whole process** — memory exhaustion, for example. Mitigated by per-job panic recovery, context timeouts, and sandboxing anything that executes.

### Neutral
- Requires more discipline in review than microservices, which enforce boundaries structurally. We consider this an acceptable trade at this team size, where review is happening anyway.

## Revisit when

- The system needs to serve multiple organisations with meaningfully different load profiles.
- One engine's resource usage genuinely requires independent scaling.
- Team size exceeds roughly 10–12 developers, where coordination cost starts to favour process boundaries.
