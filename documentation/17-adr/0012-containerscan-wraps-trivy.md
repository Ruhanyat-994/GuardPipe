# ADR-0012 — `containerscan` wraps Trivy instead of GuardPipe's own image/layer analyzer

| Status | Accepted |
|---|---|
| Date | 2026-08-14 |
| Deciders | Project owner |
| Supersedes | [ADR-0010](0010-own-scanners.md), scoped to `containerscan` only — `k8sscan`, `pentest`, and the secret sweep are unaffected and remain GuardPipe's own analyzers |

## Context

ADR-0010 decided GuardPipe would build all seven engines as purpose-built in-house analyzers, and explicitly named Trivy as the tool it was choosing *not* to wrap for `containerscan` — Trivy already runs today, but only as an independent CI hygiene check on GuardPipe's own built images (`container-scan` job, `documentation/13-devops-and-environments.md` §8.2), never as part of the product. [ADR-0011](0011-codescan-wraps-sonarqube.md) already broke from ADR-0010 once, for `codescan`, when an external requirement demanded a real third-party API integration. `containerscan` (Phase 8) had not yet been started when this was revisited a second time — same situation ADR-0011 was in for `codescan`: the least-started engine, not shipped code being discarded.

The case for `containerscan` specifically is close to what already justified `depscan` wrapping OSV.dev rather than GuardPipe maintaining its own CVE database: matching an image's installed OS packages (`dpkg`/`rpm`/`apk`) and language-level dependencies against known vulnerabilities means maintaining an accurate, continuously-updated vulnerability database across every package-manager format and every language ecosystem that could appear inside an image layer. That is not detection *logic* written once — it is a database that has to stay current forever, which is a materially different (and much larger) maintenance burden than `codescan`'s SAST rules were. Reimplementing it inside a 4-week window would produce a scanner GuardPipe could not trust its own "detection rate ≥ 90%, zero false positives on `fixture-clean`" gate on ([15 §5](../15-testing-strategy.md#5-the-golden-fixture-repositories)) — the one failure mode the whole project is designed never to produce (`CLAUDE.md`: "saying a codebase is clean when it isn't").

## Decision

`containerscan` becomes a thin wrapper + normalisation layer over Trivy, not GuardPipe's own Dockerfile-AST-plus-layer-walker pipeline described in the old `05-module-specifications.md` §8:

1. `adapters/trivy` (new adapter, same two-file shape as `adapters/sonarqube`: a thin parsing layer over Trivy's own JSON output, plus a `scanner.go` that launches a short-lived `aquasec/trivy` CLI container via `adapters/dockerx` — the same "trusted first-party tool run as a sibling container" pattern `sonar-scanner` already established, **not** built on `adapters/sandbox`, whose no-network-by-default policy exists for *untrusted* execution and would block both pulling the target image and reaching Trivy's vulnerability-database registry).
2. One Trivy invocation covers what were previously two separate hand-rolled phases:
   - **Dockerfile/config misconfiguration** (`trivy config` / `--scanners misconfig`) — runs directly against the checkout, no image build or Docker daemon required, preserving the old Phase A's "no Docker required" property.
   - **Image vulnerability + secret scanning** (`trivy image --scanners vuln,secret`) — runs against the image reference (built locally from a discovered Dockerfile, or pulled by reference), replacing GuardPipe's own `docker save` + tar-walk + package-database-read + OSV-batch-match pipeline entirely.
3. Trivy's own output is normalised into `Finding`: `RuleID` becomes `containerscan.trivy.<trivy-check-id>` for misconfig findings (e.g. `containerscan.trivy.DS002` for "no USER") and `containerscan.trivy.<CVE-id>` for vulnerability findings, `Severity` maps from Trivy's `CRITICAL/HIGH/MEDIUM/LOW/UNKNOWN` scale, `CWE` is read from Trivy's own vulnerability metadata where present, `Location` maps from the reported layer/file/package, and `Remediation` is Trivy's own fix-version/misconfig-guidance text — not AI-generated, same standing rule `codescan` already satisfies via SonarQube's copy (`CLAUDE.md`, security posture section).
4. Trivy's secret scanner covers the **built image's layers** (files and layer-history commands) — a different surface than `depscan`'s secret sweep, which covers the **git checkout** (`05-module-specifications.md` §7's "Secret sweep scope"). These don't compete for the same namespace the way SonarQube's own secret detector would have overlapped with `depscan`'s regex rules on the *same* checkout (§6's existing "not used here, to avoid two engines emitting conflicting findings" note) — an image can contain secrets that never touched git (baked in by a base image, or introduced during the build), so this is additive coverage, not a duplicate detector.
5. GuardPipe still owns and enforces its own image-size and layer-count caps (FR-CNT-011) *before* invoking Trivy — that guard doesn't depend on which tool does the analysis and stays unchanged.
6. If Trivy is unreachable (can't pull its vulnerability-database update, or the target image can't be pulled/built), only the `containerscan` job fails — `PartialResultBanner` names it, the rest of the scan completes (FR-ORC-006/NFR-REL-001, the same pattern `codescan` already uses for a SonarQube outage).
7. This does not touch or replace the existing `container-scan` CI job (`13-devops-and-environments.md` §8.2), which uses Trivy to scan **GuardPipe's own built images** as a supply-chain hygiene check — that is a CI concern about this project's own artifacts; this decision is about the `containerscan` **product engine**, which scans a client's target repository/image. Both now use Trivy, for unrelated reasons, and that CI job's job definition is unaffected.

## Rationale

The same argument ADR-0011 made for `codescan` applies here, arguably more strongly: normalisation into GuardPipe's one `Finding` model — with a stable, namespaced `RuleID`, a fingerprint that survives line/layer-number churn, and GuardPipe-controlled severity/confidence — is still real, substantial, fully-attributable engineering work regardless of which tool performs the underlying analysis. What moves is the vulnerability-matching and misconfiguration-detection logic itself, from "GuardPipe's own three-part pipeline (layer walk → package-DB read → OSV match)" to "Trivy, wrapped and filtered."

Unlike `codescan`, there is no external requirement forcing this change — it's a judgment call about where a 4-week, six-part-time-contributor team's effort is best spent. Two things make container-image vulnerability matching a worse candidate for in-house build than `codescan`'s SAST rules were:

- **The maintenance burden compounds, it doesn't just add up.** `depscan` already wraps OSV.dev rather than maintaining GuardPipe's own dependency-CVE database, for exactly this reason. `containerscan`'s old spec asked GuardPipe to independently reinvent that same class of problem — package-to-vulnerability matching — a second time, for OS packages instead of application dependencies, plus parse three different package-manager database formats (`dpkg`, `rpm`, `apk`) to get there. Trivy already solves both halves of that (the parsing *and* the aggregated vulnerability database, which itself is broader than OSV alone — it pulls from NVD, GHSA, and distro security trackers).
- **Detection recall matters more here than almost anywhere else in the product.** `CLAUDE.md`'s stated top testing priority — "saying a codebase is clean when it isn't" is the one failure mode worse than a crash — is hardest to guarantee for exactly this kind of open-ended package-database matching, where a missed advisory or a misparsed `dpkg` status file produces a silent false negative, not a visible error.

Option C from ADR-0010 (own analyzers, external tool only where the gap is largest) was rejected there specifically because "a partial dependency has the full operational cost of a dependency with only part of the benefit." That argument no longer controls once `codescan` already made this exact trade for the same reason (external database quality beating in-house recall) — the project already accepted "we built our own, except for the hard ones," as a real, honest, two-engine-not-one position, not a hypothetical inconsistency.

## Consequences

### Positive
- Trivy's vulnerability database (NVD + GHSA + distro trackers, continuously updated by a dedicated upstream project) substantially exceeds what a from-scratch OSV-only matcher would achieve for OS-package coverage — the same "detection quality" argument ADR-0011 made for SonarQube applies here.
- One tool now covers Dockerfile misconfiguration, image vulnerabilities, and image-layer secrets in a single invocation — simpler than the old three-phase (lint / layer-walk / secret-scan) design, and removes an entire hand-rolled `docker save`-and-tar-walk subsystem GuardPipe would otherwise have had to build and keep correct.
- FR-CNT-007 (application-language dependencies inside the image — `node_modules`, `site-packages`, embedded Go module data), previously Stretch because it was extra hand-rolled parsing work, is now Core-equivalent for free — Trivy already does this as part of its normal image scan.
- No separate "own vs. Trivy reference comparison" question for this engine, unlike the CI job's stated purpose (§8.2) — `containerscan` findings now come from the same tool the CI hygiene job already trusts.

### Negative
- `containerscan` is no longer demonstrable as GuardPipe's own image-analysis logic, for the same honest trade-off ADR-0011 accepted for `codescan` — the engineering contribution is the adapter, the misconfig/vuln/secret normalisation into `Finding`, and the failure-mode handling, not the underlying detection.
- New operational dependency: Trivy's CLI container has to be pulled (or pre-cached) and its vulnerability database has to be current, which needs network egress at scan time unless the database is pre-vendored on a schedule (see Revisit when).
- Trivy's own severity taxonomy, CVE coverage, and update cadence are now something GuardPipe's report quality depends on and doesn't control — same category of risk ADR-0010 flagged generally, ADR-0011 already accepted for SonarQube, now accepted here too.
- Two ADRs (0011 and this one) have now each carved one engine out of ADR-0010's original all-in-house position — `k8sscan` and `pentest` are the only two of the original seven still fully proprietary among the ones with meaningful external-tool alternatives (Checkov/kube-score, nuclei). That is a real shift in the project's overall positioning, not just a per-engine detail, and is worth the team being able to say out loud rather than letting it accumulate silently.

### Neutral
- `k8sscan`, `pentest`, and the secret-sweep rule set owned by `depscan` are entirely unaffected — ADR-0010's decision stands for them.
- The existing `container-scan` CI job (Trivy scanning GuardPipe's *own* built images, §8.2) continues unchanged and is now a coincidentally-related but operationally separate use of the same tool — worth saying explicitly so it doesn't read as redundant with this decision.

## Revisit when

- If Trivy's per-scan network dependency (pulling the image, updating its vulnerability database) proves too slow or unreliable for the demo, pre-cache/vendor the vulnerability database on a schedule instead of fetching it per scan — the actual scan invocation would then need zero network access and could move under `adapters/sandbox`'s stricter no-network harness for defense-in-depth, at the cost of a DB-freshness job to maintain.
- If GuardPipe is pursued beyond the course as a real product, the same hybrid model ADR-0011's "Revisit when" describes applies here too — own engines for correlation/scoring, external tools (Trivy included) for raw detection where their database quality matters more than attribution.
