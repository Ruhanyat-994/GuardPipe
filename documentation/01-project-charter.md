# 01 — Project Charter

| Field | Value |
|---|---|
| **Document** | Project Charter |
| **Project** | GuardPipe |
| **Version** | 1.0 |
| **Status** | Draft |
| **Authors** | GuardPipe Team |
| **Last updated** | 2026-07-29 |

### Revision history

| Version | Date | Author | Change |
|---|---|---|---|
| 1.0 | 2026-07-29 | Team | Initial charter |

---

## 1. Executive summary

GuardPipe is a **software supply-chain security platform** that inspects every stage of the software development lifecycle — from design documents through source code, dependencies, container images, Kubernetes manifests, and CI/CD pipelines — and concludes with an automated penetration-testing pass. Findings from all stages are normalised into a single model, scored into one **GuardPipe Risk Score**, and presented in an interactive dashboard where each issue carries its CVE/CWE reference and an AI-generated remediation suggestion.

The platform answers a single question that no existing tool answers end-to-end:

> **Can this software safely reach production?**

---

## 2. Problem statement

Modern software passes through a chain of disconnected security tools:

- SAST tools see source code but not the container it ships in.
- Dependency scanners see the manifest but not how the package is used.
- Container scanners see OS packages but not the Kubernetes RBAC that will run them.
- IaC checkers see manifests but not the CI/CD pipeline that applies them.
- Pentest reports arrive weeks later, disconnected from the code that caused the issue.

The result: each tool shows a **fragment of risk**, in a different format, with a different severity scale, and nobody can answer whether the release as a whole is safe. Teams either drown in noise or ship blind.

This gap is not academic. The most damaging incidents of the last several years — compromised build systems, malicious dependency updates, leaked CI secrets, over-permissioned service accounts — all lived in the *seams between* tools, not inside any one of them.

## 3. Vision

GuardPipe unifies the seams. One platform, one data model, one risk score, one dashboard, covering:

```
Requirements → Design Docs → Code → Dependencies → Container → Kubernetes → CI/CD → Runtime Pentest
     └───────────────────── all inspected by GuardPipe ─────────────────────┘
```

**Positioning.** GuardPipe is not a re-implementation of Snyk, Trivy, or SonarQube. It is a **decision engine**: it runs its own purpose-built analyzers, normalises what they find, correlates findings across stages, and produces a defensible go/no-go verdict with AI-authored explanations and patches.

---

## 4. Objectives

| # | Objective | Measure of success |
|---|---|---|
| O1 | Cover all seven SDLC security stages in one product | 7 scan engines operational and reachable from one UI |
| O2 | Produce a single, explainable risk verdict | Risk score 0–100 with a per-engine breakdown the user can drill into |
| O3 | Make findings actionable, not just visible | ≥ 80% of findings carry a CWE/CVE and an AI patch suggestion |
| O4 | Let users run either the full chain or one stage | Both "Full Supply Chain Scan" and per-engine scans available |
| O5 | Be safe to run against untrusted repositories | All shell/pentest execution sandboxed; documented threat model |
| O6 | Be demonstrable end-to-end in 10 minutes | Rehearsed demo script completes without manual intervention |

---

## 5. Scope

### 5.1 In scope (this semester)

| Stage | Engine | Delivered capability |
|---|---|---|
| Design | `docreview` | AI review of design/requirement docs: spelling, clarity, contradictions, missing security considerations, questionable architectural decisions |
| Code | `codescan` | Own static analyzer: SQL injection, XSS, command injection, path traversal, hardcoded secrets, weak crypto, insecure deserialization, and more |
| Code | `depscan` | Dependency inventory + known-vulnerability lookup (OSV.dev), plus secret detection across the repo |
| Build | `containerscan` | Dockerfile linting + image layer inspection + OS/language package inventory + CVE matching |
| Deploy | `k8sscan` | Kubernetes manifest policy analysis: RBAC over-permission, privileged/root containers, hostPath mounts, missing NetworkPolicy, Pod Security Admission level |
| Pipeline | `cicdscan` | GitHub Actions workflow analysis: unpinned actions, `pull_request_target` misuse, secret exposure, over-broad `GITHUB_TOKEN` permissions — rule-based **plus** AI review |
| Runtime | `pentest` | Orchestrated bash penetration-testing suite against an authorised target: port/service discovery, TLS posture, HTTP security headers, common misconfigurations |
| All | `ai` | Gemini-powered explanation, patch generation, and pipeline analysis |
| All | `scoring` + dashboard | Unified risk score, findings explorer, per-finding detail with CVE and patch diff |

### 5.2 Out of scope (explicitly not this semester)

| Excluded | Why |
|---|---|
| Runtime intrusion detection / eBPF agents | Requires cluster-resident agents; far beyond a 4-week window |
| SBOM cryptographic signing (Sigstore/cosign) | Depends on key infrastructure; recorded as future work |
| Compliance report generation (SOC 2, ISO 27001) | Needs auditable evidence chains; not demonstrable at this scale |
| Multi-cloud IaC (Terraform, Helm, Ansible, CloudFormation) | Kubernetes YAML only for now |
| Live Kubernetes cluster connection | Manifest analysis only — no kubeconfig ingestion |
| Multi-tenant SaaS billing, SSO, org hierarchy | Single-organisation model is sufficient |
| Autonomous exploitation in `pentest` | Deliberate safety boundary — see [12 — Security & Threat Model](12-security-and-threat-model.md) |

### 5.3 Future product vision (documented, not built)

Runtime anomaly detection · full supply-chain trust scoring with signed SBOMs · compliance engine (OWASP/CIS/NIST/SOC 2) · executive dashboards and automated release gates · multi-cloud IaC coverage · predictive organisation-wide risk intelligence.

---

## 6. Stakeholders

| Stakeholder | Interest | Influence |
|---|---|---|
| Course instructor | Correctness, engineering rigour, demonstrated learning outcomes | **Decision authority** — accepts or rejects the deliverable |
| Project team (6) | Deliver working software; learn Go, security engineering, team process | High |
| Team Lead | Scope control, integration, demo readiness | High |
| End-user persona: **DevSecOps Engineer** | Wants one place to see all risk before a release | Primary user |
| End-user persona: **Developer** | Wants a clear fix, not a lecture | Primary user |
| End-user persona: **Engineering Manager** | Wants a yes/no release verdict and a trend | Secondary user |

---

## 7. Team and ownership

Six contributors. Each owns a vertical slice — a module plus its API surface, database tables, tests, and documentation section. Ownership means *accountable for it working on demo day*, not *the only person allowed to touch it*.

| # | Name | Student ID | Role | Owns (modules) |
|---|---|---|---|---|
| 1 | *[Name]* | *[ID]* | Team Lead / Backend | `identity`, `project`, `orchestrator`, shared DB schema, release management |
| 2 | *[Name]* | *[ID]* | Backend — Code Security | `codescan`, `depscan` |
| 3 | *[Name]* | *[ID]* | Backend — Infrastructure Security | `containerscan`, `k8sscan` |
| 4 | *[Name]* | *[ID]* | Backend — AI & Pipeline | `ai`, `docreview`, `cicdscan` |
| 5 | *[Name]* | *[ID]* | Frontend Lead | React SPA, dashboard, findings explorer |
| 6 | *[Name]* | *[ID]* | DevOps / QA / Design | Docker Compose, CI, `pentest` sandbox runner, Figma design system, test strategy |

> Fill in names and IDs before the first PR. The same table appears in [16 — Project Plan](16-project-plan.md) as a RACI matrix and in [14 — GitHub Workflow](14-github-workflow.md) as the `CODEOWNERS` mapping — keep all three in sync.

---

## 8. Constraints

| Constraint | Impact |
|---|---|
| **Build window: 3–4 weeks** (start 2026-08-03, demo 2026-08-28) | Drives the modular-monolith decision and the Core/Stretch rule tiers |
| **6 contributors, part-time, varying Go experience** | Module boundaries must be strict; shared-schema changes need a protocol |
| **Zero budget** | Gemini free tier; no paid scanners; no cloud hosting — Docker Compose locally |
| **Must use Go with a backend framework** | Go + Gin — see [ADR-0002](17-adr/0002-go-and-gin.md) |
| **Must use a database** | PostgreSQL — see [ADR-0003](17-adr/0003-postgresql-and-redis.md) |
| **Must produce an SRS and a Figma design** | [02 — SRS](02-srs.md), [09 — UI/UX & Design System](09-ui-ux-design-system.md) |
| **Docker required on dev machines** | For sandboxed scan execution and Compose |

---

## 9. Assumptions

1. Every team member has Docker Desktop, Go 1.23+, Node 20+, and a GitHub account.
2. Target repositories for scanning are public GitHub repos, or private repos with a supplied PAT.
3. Gemini free-tier quota is sufficient for development and one live demo; results are cached by content hash to conserve it.
4. Penetration testing is only ever run against targets the user attests they own — enforced in product, not just in policy.
5. The demo runs on a laptop with Docker Compose; no cloud deployment is required for acceptance.

---

## 10. Success criteria (definition of done for the project)

The project is complete when **all** of the following hold:

- [ ] A user can register, log in, and create a project.
- [ ] A user can run a **full supply-chain scan** on a GitHub repository and see all seven stages report.
- [ ] A user can run **any single engine** independently.
- [ ] A user can run a **standalone penetration test** against an authorised target.
- [ ] Findings appear in a filterable explorer with severity, CWE/CVE, file/line or resource location.
- [ ] Opening a finding shows an AI explanation and a suggested patch as a diff.
- [ ] The dashboard shows a unified 0–100 risk score with a per-engine breakdown.
- [ ] The system detects seeded vulnerabilities in the intentionally-vulnerable fixture repository (golden tests pass).
- [ ] CI is green on `main`: lint, test, build, and GuardPipe scanning its own repository.
- [ ] All 18 documents in this folder are `Approved`.
- [ ] The 10-minute demo script runs end-to-end without manual database edits.

---

## 11. Top risks (summary — full register in [16 — Project Plan](16-project-plan.md))

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Scope too large for 4 weeks | **High** | High | Core/Stretch rule tiers; feature freeze 2026-08-24; engines degrade gracefully |
| Gemini quota exhausted mid-demo | Medium | High | Aggressive response caching; pre-warmed demo cache; rule-based fallback path |
| Six people, one schema → migration conflicts | **High** | Medium | Schema change protocol + 2-approval rule; sequential migration numbering |
| Uneven Go experience blocks parallel work | Medium | High | Sprint 0 scaffolds all module skeletons so nobody starts from a blank file |
| Sandbox escape from a hostile scanned repo | Low | High | No-network, read-only, resource-capped, non-root, time-limited containers |
| Integration left to the last week | Medium | High | Vertical slice (`depscan` end-to-end) completed in Sprint 1 |

---

## 12. Approval

| Role | Name | Date | Signature |
|---|---|---|---|
| Team Lead | *[Name]* | | |
| Course Instructor | *[Name]* | | |
