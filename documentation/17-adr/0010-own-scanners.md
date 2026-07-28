# ADR-0010 — Build our own scanners rather than wrapping existing tools

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | Full team |
| Supersedes | — |

## Context

Mature open-source tools exist for most of what GuardPipe does: Semgrep and CodeQL for SAST, Trivy and Grype for containers and dependencies, Checkov and kube-score for Kubernetes, Gitleaks for secrets, nuclei for network checks.

Wrapping them would be faster and would produce better detection. The alternative is implementing our own analyzers.

The forces:
- **This is a software engineering deliverable.** The learning outcome is building the thing, not integrating it.
- The user's brief explicitly asked for own tooling for the container, Kubernetes, and pentest engines.
- Four weeks, six people.
- Wrapping N binaries means shipping and updating N binaries, each with its own output format, exit codes, and failure modes.
- Detection quality matters for credibility.

## Options considered

### Option A — Wrap existing tools
GuardPipe becomes an orchestration and normalisation layer over Semgrep, Trivy, Checkov, and nuclei.

### Option B — Build our own analyzers
Purpose-built engines in Go, no external scanner binaries.

### Option C — Hybrid
Own analyzers, with an external tool where the gap is largest.

## Decision

**Option B — build our own analyzers.** No external scanner binary is a runtime dependency of any engine.

External advisory *data* (OSV.dev, NVD) is used — that is a database, not a scanner. Trivy is used **in CI** as an independent check on our own images, but is not part of the product.

## Rationale

The honest framing is that Option A would produce a better *scanner* and a worse *project*. Wrapping Trivy means the container engine is Trivy; the engineering contribution is a JSON adapter. For an academic deliverable whose purpose is demonstrating that we can build this, that hollows out the centre of the work.

There are also real engineering arguments beyond the academic one:

**Normalisation is genuinely hard, and it is where wrapping leaks.** Every tool has its own severity scale, its own location format, its own confidence semantics, and its own notion of a rule identity. Mapping four such models into one coherent `Finding` — with a stable fingerprint that survives across scans — turns out to be a substantial share of the work anyway. Building the analyzers to emit the model directly avoids an entire lossy translation layer.

**Operational cost.** Wrapping means shipping four binaries in the image, keeping four versions current, handling four sets of exit codes and error formats, and debugging four upstream projects' behaviour changes mid-sprint.

**Scope control makes it tractable.** We are not attempting to match Semgrep. The Core rule tiers ([05 §16](../05-module-specifications.md#16-rule-count-summary)) define 112 rules plus ~35 pentest checks across seven engines — a scoped, demo-credible subset chosen for coverage of the classes that matter most. Go's standard library does much of the heavy lifting: `go/parser` gives real AST analysis for Go at zero cost, `archive/tar` reads image layers, `sigs.k8s.io/yaml` parses manifests.

Option C was rejected because a partial dependency has the full operational cost of a dependency with only part of the benefit, and it makes the story inconsistent — "we built our own, except for the hard one."

**The cost is stated honestly and measured.** Our detection recall will be below mature tools. Rather than obscuring that, we measure it against golden fixtures and publish the number ([15 §5](../15-testing-strategy.md#5-the-golden-fixture-repositories)): detection rate ≥ 90% against a catalogued fixture, zero false positives on a clean fixture, with limitations documented in-product. A tool that knows and states its own coverage is more trustworthy than one that implies completeness.

## Consequences

### Positive
- The engineering contribution is real and defensible.
- Findings are produced directly in our model — no lossy translation, no impedance mismatch.
- Complete control over severity, confidence, remediation text, and fingerprinting.
- No external binaries in the image; a ~25 MB distroless container.
- No third-party scanner failure modes, version drift, or output-format changes to absorb.
- Full understanding of every rule — we can explain any finding to an evaluator.
- Deep learning outcome in static analysis, container internals, RBAC semantics, and CI/CD attack patterns.

### Negative
- **Lower detection recall than mature tools.** This is the real cost. Mitigated by scoped Core tiers, measured detection rates, and published limitations.
- **False positives are our problem to solve**, with no upstream tuning to inherit. Mitigated by near-miss tests for every rule and a `fixture-clean` zero-false-positive CI gate.
- Substantially more implementation work — the dominant share of Sprints 1 and 2.
- No taint analysis for JS/TS/Java/PHP beyond Tier 2; stated as a known limitation rather than hidden.
- We maintain our own rule set with no community contributions.

### Neutral
- Using Trivy in CI while building our own container scanner is not a contradiction: it is an independent control on our own supply chain and a useful reference to compare our output against. This is stated explicitly in [13 §8.2](../13-devops-and-environments.md#82-jobs) so it does not read as an inconsistency.

## Revisit when

- GuardPipe is pursued beyond the course as a real product → a hybrid model, using our engines for correlation and scoring while optionally ingesting SARIF from external tools, becomes the sensible commercial architecture.
- A specific engine's recall proves inadequate for a real use case.
