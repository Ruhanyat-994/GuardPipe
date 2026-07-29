# ADR-0005 — In-process Go analysis with Docker-sandboxed shell execution

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M6, M1, full team |
| Supersedes | — |

## Context

GuardPipe analyses repositories it does not control. Two engines need capabilities beyond reading files:

- `pentest` runs a bash suite that makes network requests to an external target.
- `containerscan` extracts and inspects container image layers.

The remaining five engines only parse files.

Threats to address:
- A crafted repository exploiting a parser (zip bombs, deeply nested structures, path traversal).
- Untrusted shell execution on the host.
- A pentest script bug producing unintended network activity.
- Resource exhaustion from either.

## Options considered

### Option A — Everything in-process Go
No Docker dependency. Simplest to build and demo.

### Option B — In-process Go analysis + Docker sandbox for shell and image work
Pure-Go analyzers run in the application process; anything executing a command runs in a disposable container.

### Option C — Everything in the sandbox
Every engine runs in a container.

### Option D — A separate worker binary
Second process sharing the queue and database.

## Decision

**Option B.** Pure-Go static analysis runs in-process. Everything that executes a command — the pentest suite, image layer extraction — runs in an ephemeral, resource-capped, network-restricted, non-root container.

## Rationale

The distinction that matters is **parsing untrusted data** versus **executing untrusted-adjacent code**.

Parsing is comparatively safe in Go: memory safety eliminates the buffer-overflow class outright, and RE2 (Go's regex engine) has no catastrophic backtracking, eliminating ReDoS — the two exploit classes that would otherwise argue for isolating the parsers. Residual risks (zip bombs, deep nesting, path traversal) are addressed with explicit limits and canonicalisation, not with process isolation.

Executing shell commands against an external target is a different category entirely. A bug in a bash script that runs on the host has host privileges; the same bug in a container with `cap-drop=ALL`, a read-only root filesystem, `pids-limit`, memory caps, and a network restricted to a single pinned IP is contained. Isolation is not optional there.

Option A was rejected specifically because of `pentest`. Running arbitrary network-touching bash on the host, with no rate enforcement outside the script itself, and no containment if the script misbehaves, is exactly the class of decision a security product must not make. The defence-in-depth requirement in [12 §5.3](../12-security-and-threat-model.md#53-prohibited-activity--enforced-in-code-not-policy) — rate limits enforced in *two independent layers* — is only achievable with a sandbox.

Option C was rejected on cost and benefit: containerising the five pure-Go engines adds container startup latency to every scan, complicates streaming findings back to the orchestrator, and defends against threats that Go's memory safety already handles.

Option D is architecturally reasonable but adds a second deployable, working against the modular-monolith decision ([ADR-0001](0001-modular-monolith.md)) for no benefit we currently need. The `GUARDPIPE_ROLE=worker` split keeps this available later at near-zero cost.

**The accepted risk** is stated plainly in [12 §4](../12-security-and-threat-model.md#4-the-docker-socket--our-largest-accepted-risk): mounting the Docker socket grants the application host-equivalent privilege. An RCE in GuardPipe escalates to host compromise. We accept this because the deployment target is a local development or demo machine, and we document rootless Docker or a dedicated sandbox daemon as the production remediation. Hiding it would be worse than the risk itself.

## Consequences

### Positive
- Untrusted execution is contained: non-root, no capabilities, read-only filesystem, restricted network, resource caps, hard timeout.
- Rate limiting on pentest activity is enforced independently of the scripts.
- Five engines have zero container overhead and stream findings directly.
- Engines remain trivially unit-testable — they are pure functions over a directory.
- No Docker dependency for the majority of functionality: without Docker, `containerscan` Phase A and all other engines still work.

### Negative
- **Docker socket mount = host-equivalent privilege.** The single largest accepted risk in the product.
- Docker is required for two engines; without it they are `skipped`, not failed.
- Container startup adds 1–3 seconds to sandboxed jobs.
- Orphaned containers are possible after a crash — mitigated by a startup sweep and `defer` force-removal.
- The team must understand two execution models rather than one.

### Neutral
- The `Sandbox` interface makes the execution mechanism replaceable — a rootless daemon or a gVisor runtime would be a drop-in change.

## Revisit when

- The system is deployed anywhere other than a local machine → rootless Docker becomes mandatory, not optional.
- A pure-Go image-layer reader removes `containerscan`'s need for the socket.
- A parser vulnerability is found in practice → reconsider isolating the parsing engines too.
