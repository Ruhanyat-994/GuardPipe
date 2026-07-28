# ADR-0002 — Go with the Gin web framework

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | Full team |
| Supersedes | — |

## Context

Go is a project requirement, not a choice. The decision here is which HTTP framework to build the modular monolith on.

Requirements:
- Straightforward middleware composition — we need eight middleware layers ([04 §4.1](../04-backend-architecture.md#41-middleware-chain--order-matters)).
- Learnable in under a day by developers with limited Go experience.
- Standard `net/http` semantics, so the wider ecosystem (`httptest`, standard middleware, testing libraries) works without adaptation.
- Actively maintained.
- No code generation step in the development loop.

## Options considered

### Option A — Gin
Most-used Go web framework. Radix-tree router, familiar `gin.Context`, very large ecosystem and tutorial base.

### Option B — Echo
Similar scope to Gin, arguably cleaner middleware signature and better binding ergonomics.

### Option C — Fiber
Built on `fasthttp`. Fastest in benchmarks, Express-like API.

### Option D — chi + standard library
Minimal router over pure `net/http`. Most idiomatic Go.

### Option E — go-zero
Full framework with code generation from `.api` and `.proto` definitions. Proposed by the earlier plan.

## Decision

**Option A — Gin.**

## Rationale

Gin and Echo are close on merit. Gin wins on the factor that matters most in a four-week project with mixed experience: **the volume of accurate learning material.** When a teammate is stuck on binding, middleware ordering, or file uploads at 11 pm, Gin has an answer within one search. That is not an elegant reason, but it is the correct one under this constraint.

Fiber was rejected on a technical concern rather than taste: `fasthttp` does not implement `net/http` interfaces, so `httptest`, standard middleware, and a number of testing libraries need adapters. In a project where testing discipline is central to the deliverable ([15 — Testing Strategy](../15-testing-strategy.md)), friction in the test path is expensive. Its performance advantage is irrelevant — our latency is dominated by scan execution, not HTTP routing.

chi is the choice we would make for a long-lived production service. Here, the extra boilerplate it requires would be written six different ways by six people in week one.

go-zero was rejected with the microservices architecture it belongs to ([ADR-0001](0001-modular-monolith.md)). Its value is in generating service scaffolding for a distributed system; in a monolith it adds a code-generation step to the development loop and buys nothing.

Framework lock-in is limited by design: Gin appears only in `transport/http`. Services take `context.Context` and typed inputs, never `*gin.Context` ([04 §2.1](../04-backend-architecture.md#21-layer-rules--non-negotiable)). Replacing Gin would touch one package.

## Consequences

### Positive
- Fastest path to a working API for this team.
- Enormous middleware ecosystem — CORS, rate limiting, request ID all available.
- Built-in binding and validation integration with `go-playground/validator`.
- Standard `net/http` semantics; `httptest` works directly.
- Every teammate can find help independently.

### Negative
- `gin.Context` conflates request context, response writer, and a key-value store — mitigated by never letting it past the transport layer.
- Gin's error handling (`c.Error()` plus a collector) is idiosyncratic — mitigated by a single `ErrorMapper` middleware that is the only place HTTP error bodies are constructed.
- Marginally slower than Fiber. Irrelevant at our scale.

### Neutral
- Middleware order becomes load-bearing and must be documented — done in [04 §4.1](../04-backend-architecture.md#41-middleware-chain--order-matters).

## Revisit when

- The transport layer becomes a measured bottleneck (it will not).
- Gin maintenance stalls.
- A future extraction to services makes a different framework natural for the extracted piece.
