# ADR-0011 — `codescan` wraps a self-hosted SonarQube instead of GuardPipe's own SAST

| Status | Accepted |
|---|---|
| Date | 2026-08-14 |
| Deciders | Project owner (external requirement) |
| Supersedes | [ADR-0010](0010-own-scanners.md), scoped to `codescan` only — `containerscan`, `k8sscan`, `pentest`, and the secret sweep are unaffected and remain GuardPipe's own analyzers |

## Context

ADR-0010 decided GuardPipe would build all seven engines as purpose-built in-house analyzers, explicitly rejecting wrapping Semgrep/CodeQL-class SAST tools for `codescan`. Phase 7 (`codescan`) had not yet been started when this reversed: an external requirement on the project (the grading brief) now requires GuardPipe to demonstrably integrate a real third-party API rather than being 100%-in-house end to end.

`codescan` is the natural place for this: it is the one engine where a mature, free, self-hostable external scanner (SonarQube Community Edition) already does the job well, and it was the least-started engine (Phase 7, not yet built) when the requirement changed — so this is a redesign, not a rewrite of shipped code.

## Decision

`codescan` becomes a thin wrapper + filter over a self-hosted SonarQube Community Edition instance, not GuardPipe's own static analyzer:

1. `adapters/sonarqube` (new adapter, same pattern as `adapters/osv`/`adapters/github`) drives SonarQube's Web API: triggers a `sonar-scanner` analysis of the cloned repo (or invokes it via `docker/docker/client` the same way `adapters/sandbox` runs pentest scripts), polls `/api/qualitygates/project_status` for completion, then reads `/api/issues/search` and `/api/hotspots/search` for the analyzed project.
2. `engines/codescan` filters SonarQube's raw output to **security-relevant findings only** — `type=VULNERABILITY` and `SECURITY_HOTSPOT` — and discards code smells, maintainability, and coverage-adjacent output. SonarQube's general code-quality output is never surfaced to the client; GuardPipe is a security tool, not a code-quality dashboard.
3. Each surviving SonarQube issue/hotspot is normalised into GuardPipe's `Finding` model: `RuleID` becomes `codescan.sonarqube.<sonar-rule-key>` (permanent once assigned, per the existing fingerprint rule in `03-architecture-overview.md` §7.1), `Location` maps from SonarQube's `component`/`textRange`, `Severity` maps from SonarQube's `severity`/hotspot `vulnerabilityProbability`, `CWE` is read from the rule's security-standard tags where SonarQube provides one.
4. **Remediation text comes from SonarQube's own rule description ("How to fix it" section of `/api/rules/show`)**, not from GuardPipe's AI module. This still satisfies the standing rule that `Finding.Remediation` must stand alone without AI (`CLAUDE.md`, security posture section) — it's just sourced from SonarQube's copy instead of hand-written GuardPipe copy. `modules/ai` can still enrich the finding with an AI explanation later exactly as it does for every other engine's findings; that path is unchanged.
5. If SonarQube is unreachable, times out, or the analysis fails, only the `codescan` job fails/skips (`FR-ORC-006`/`NFR-REL-001`, the existing "one failed job never fails the whole scan" contract) — `PartialResultBanner` names it, the rest of the scan completes. SonarQube is a dependency of one engine, not of the orchestrator.
6. SonarQube runs self-hosted via a `sonarqube` service in `docker-compose.yml` (Community Edition, with its own dedicated Postgres database service) — not SonarCloud — to keep the zero-budget/local-only constraint (`01-project-charter.md` §8) intact and to support private repos, not just public ones.

## Rationale

ADR-0010's core argument — normalisation is genuinely hard and wrapping leaks that work back onto GuardPipe anyway — still holds and is exactly why this integration is a filter-and-normalise adapter rather than a passthrough: SonarQube's own issue/hotspot model still has to be mapped into the one `Finding` shape every other engine produces, with a stable fingerprint, a namespaced rule ID, and a severity mapping GuardPipe controls. The engineering work ADR-0010 called "where wrapping leaks" doesn't disappear — it moves from "write the analyzer" to "write the adapter + the security-relevance filter + the normalisation," which is still real, non-trivial work, and still fully attributable to this project.

What changes is the analysis engine underneath: SonarQube's actual rule execution replaces GuardPipe's own three-tier regex/AST/taint engine described in the old `05-module-specifications.md` §6. That tier system was **specification only, never implemented** (Phase 7 was `[ ]` unchecked in `BUILD_GUIDE.md` when this ADR was written) — there is no working code being discarded.

## Consequences

### Positive
- Real third-party API integration, satisfying the external requirement.
- SonarQube's detection quality substantially exceeds what a 4-week from-scratch SAST engine would have achieved — the "lower detection recall than mature tools" negative from ADR-0010 goes away for this engine specifically.
- SonarQube's own remediation text is generally higher quality and more consistently maintained than hand-written rule-by-rule remediation strings would have been.
- Multi-language coverage (SonarQube supports far more languages out of the box) without GuardPipe hand-building a parser per language.

### Negative
- `codescan` is no longer demonstrable as GuardPipe's own analysis logic — the engineering contribution here is the adapter, the security-relevance filter, and the normalisation layer, not the detection logic itself. This is the honest trade-off ADR-0010 originally avoided; it's accepted here because it's now an explicit external requirement, not a shortcut taken to save time.
- New operational dependency: a SonarQube container (plus its own Postgres database) has to run, stay healthy, and be reachable for `codescan` to produce anything. Mitigated by the engine-level (not scan-level) failure handling in Decision point 5.
- SonarQube Community Edition's own rule set, severity taxonomy, and update cadence are now something GuardPipe's report quality depends on and doesn't control — same category of risk ADR-0010 flagged for Option A generally, now accepted for this one engine.
- `codescan` gets an external brand mark (`SonarQubeMark`) where the original design (`09-ui-ux-design-system.md` §4.5) deliberately gave it none, on the reasoning that it was GuardPipe's own engine. That reasoning no longer applies; the mark is now honest, not misleading, per the same "this is what makes the graph read as GuardPipe talking to real tools" principle already applied to `depscan`/OSV, `containerscan`/Docker, `k8sscan`/Kubernetes.

### Neutral
- `containerscan`, `k8sscan`, `pentest`, and the secret-sweep rule set (reused by `depscan`, per `05-module-specifications.md` §7's "Secret sweep scope" note) are entirely unaffected — ADR-0010's decision stands for all of them. This is a single-engine exception, not a reversal of the project's overall in-house-analyzer positioning.

## Revisit when

- If the external-API requirement that motivated this is lifted and detection-recall/attribution concerns from ADR-0010 outweigh it, `codescan` could revert to (or run alongside) an in-house Tier 1/2 analyzer using the original §6 spec as a starting point — it's preserved in this ADR's history and in `02-srs.md`'s revision history, not deleted.
- If SonarQube's operational cost (container memory, startup time, flakiness) becomes a recurring problem in CI or local dev, consider SonarCloud for CI only while keeping self-hosted for local dev, or dropping SonarQube in favour of a lighter external scanner.
