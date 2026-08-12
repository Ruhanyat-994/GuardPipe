# GuardPipe — Team Task Split (Phase 4 onward)

> **Personal planning file, gitignored, never pushed** — same category as `BUILD_GUIDE.md`/`PROGRESS-LOG.md`. Mirrors the Jira board (`SCRUM-44` → `SCRUM-71`) into `BUILD_GUIDE.md`'s phase order so each teammate's slice reads against the same spec references the rest of the build already uses. Update the checkboxes here as tickets move on the board — this file doesn't sync automatically.

## Roster

Fill in real names/GitHub handles against the Jira avatar initials once, then every phase section below just refers back to the initial.

| Initial | Name | GitHub handle | Jira avatar colour |
|---|---|---|---|
| MR | *(you)* | `Ruhanyat-994` | blue |
| IH | | | purple |
| GH | | | green |
| MH | | | red |
| M | | | grey |
| EM | | | cyan |

## How to use this per teammate

For each ticket below: create a branch off `main` named per `documentation/14-github-workflow.md` (`<type>/<module>/<short-kebab-description>`, e.g. `feat/advisory/osv-client`), open a PR referencing the `SCRUM-###` ticket in the title or description, and get it reviewed per the module's CODEOWNERS requirement (two approvals for schema/AI/sandbox/pentest/contract docs, one otherwise). Doc references point at the authoritative spec section — read that section before starting, not just this row.

---

## Phase 4 — AI shared adapter

*`BUILD_GUIDE.md` Phase 4 · needed before any AI-powered engine*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-44 | IH | Aug 12 | `modules/ai` `LLMProvider` port + `adapters/gemini` + prompt registry/schema validation + `prompt_injection_attempt` emission + content-hash caching + **multi-key rotation pool** (`GUARDPIPE_GEMINI_API_KEYS`, 429 → advance key → retry once) | `feat/ai/gemini-shared-adapter` | `documentation/10-ai-integration.md` §5,§7; `documentation/13-devops-and-environments.md` §5; `BUILD_GUIDE.md` Phase 4 |

**Done when:** a throwaway CLI command sends a prompt through the fake `LLMProvider` in tests and through real Gemini manually; a faked 429 on key 1 provably retries on key 2. No frontend this phase (nothing calls `modules/ai` yet).

---

## Phase 5 — Advisory data and the sandbox

*`BUILD_GUIDE.md` Phase 5*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-45 | MR | Aug 17 | `modules/advisory`: OSV.dev client + Redis-backed cache; migration `00006` (`rules` table); `GET/PATCH /rules*` endpoints | `feat/advisory/osv-client-and-rules` | `documentation/06-database-design.md` §4.15/§11; `documentation/07-api-specification.md` §8 |
| SCRUM-46 | MR | Aug 17 | `adapters/dockerx` (thin Docker SDK wrapper) + `adapters/sandbox` (container lifecycle, timeouts, resource limits, cleanup, startup orphan-sweep), tests tagged `-tags=docker` | `feat/sandbox/dockerx-and-sandbox-adapter` | `documentation/12-security-and-threat-model.md`; `BUILD_GUIDE.md` Phase 5 |
| SCRUM-47 | MR | Aug 17 | Frontend Rules catalogue page (`/rules`, replaces Phase 3 placeholder): filterable list, detail view, admin-only enable/disable toggle | `feat/frontend/rules-catalogue-page` | `documentation/09-ui-ux-design-system.md` §5.8 (Screen 11) |

**Done when:** a test spins up a throwaway container, enforces a timeout, and confirms cleanup; `/rules` in the browser shows real advisory data instead of the old placeholder.

---

## Phase 6 — First engine end to end: `depscan`

*`BUILD_GUIDE.md` Phase 6 · proves the whole architecture before the other six engines*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-48 | GH | Aug 21 | Migrations `00005` (`scans`/`scan_jobs`), `00007` findings-half (`findings`/`finding_evidence`), `00009` dependency-half (`dependencies`/`scan_evidence`) | `schema/store/scan-findings-dependency-migrations` | `documentation/06-database-design.md` §4.9/§4.10/§11 |
| SCRUM-49 | GH | Aug 21 | Migration `00010` (`audit_log`) **+ retroactive instrumentation** of already-shipped Phase 2/3 code (`auth.login`, `auth.logout`, `refresh.reuse_detected`, `project.created`, `project.archived`, `repository.attached`, `target.attested`) | `feat/store/audit-log-and-retro-instrumentation` | `documentation/06-database-design.md` §4.19; `BUILD_GUIDE.md` Phase 6 gap-check note |
| SCRUM-50 | GH | Aug 21 | `engines/depscan` (per-ecosystem dependency parsers + secret sweep + OSV lookup) + `modules/orchestrator` minimal version (scan creation, Redis job queue, worker pool, engine registry wired to `depscan` only) + one-transaction-per-job findings persistence + end-to-end fixture test | `feat/orchestrator/depscan-and-minimal-orchestrator` | `documentation/05-module-specifications.md` §5-6; `documentation/03-architecture-overview.md` §7.1 (`Engine` interface) |
| SCRUM-51 | MH | Aug 21 | Frontend: scan-trigger button on project view (minimal single-button, full wizard is Phase 12) + `SupplyChainPipeline` live execution graph (GitHub-Actions-style connected nodes, polls `GET /scans/{id}/progress`) + `OsvMark` on the `depscan` node/findings | `feat/frontend/scan-trigger-and-live-graph` | `documentation/09-ui-ux-design-system.md` §4.8, §4.5; `documentation/07-api-specification.md` §5 |
| SCRUM-52 | MH | Aug 21 | Frontend: `PartialResultBanner` (first phase a partial result is possible) + minimal raw findings list on the completed scan (no filter/sort/pagination yet) | `feat/frontend/partial-result-banner-and-findings-list` | `documentation/09-ui-ux-design-system.md` §4.2 |

**Done when:** `POST /projects/{id}/scans` → poll → completed scan with real `depscan` findings in Postgres, **and** the browser can trigger that scan, watch the live graph animate `depscan` queued→running→done, and see findings appear.

---

## Phase 7 — `codescan`

*`BUILD_GUIDE.md` Phase 7*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-53 | M | Aug 26 | GuardPipe's own SAST rules: SQLi, XSS, command injection, path traversal, hardcoded secrets, weak crypto, insecure deserialization — true-positive **and** near-miss test per rule, fixture planted in the same PR | `feat/codescan/sast-rule-set` | `documentation/05-module-specifications.md` §6; `documentation/15-testing-strategy.md` §5 |
| SCRUM-54 | M | Aug 26 | Frontend: register `codescan`'s node in `SupplyChainPipeline` (label + `EngineIcon` only — no brand mark, it's GuardPipe's own engine) | `feat/frontend/register-codescan-node` | `BUILD_GUIDE.md` Phase 7 |

**Done when:** `codescan` findings appear in a real scan, and its node in the live graph shows real status instead of `not_run`.

---

## Phase 8 — `containerscan`

*`BUILD_GUIDE.md` Phase 8*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-55 | EM | Aug 31 | Dockerfile lint (no Docker required) + image layer/package inspection via `adapters/sandbox` + `modules/advisory` — true-positive/near-miss test per rule, fixture planted same PR | `feat/containerscan/dockerfile-lint-and-image-analysis` | `documentation/05-module-specifications.md` §8 |
| SCRUM-56 | MH | Aug 31 | Frontend: register `containerscan`'s node in `SupplyChainPipeline` + `DockerMark` on the node and container/image-layer findings | `feat/frontend/register-containerscan-node` | `documentation/09-ui-ux-design-system.md` §4.5 |

**Done when:** `containerscan` findings appear in a real scan (Dockerfile lint alone is enough to demo before a full image-analysis fixture exists).

---

## Phase 9 — `k8sscan`

*`BUILD_GUIDE.md` Phase 9*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-57 | MR | Sep 4 | Manifest policy rules: RBAC over-permission, privileged/root containers, hostPath mounts, missing NetworkPolicy, Pod Security Admission level — true-positive/near-miss test per rule, fixture planted same PR | `feat/k8sscan/manifest-policy-rules` | `documentation/05-module-specifications.md` §9 |
| SCRUM-58 | MH | Sep 4 | Frontend: register `k8sscan`'s node in `SupplyChainPipeline` + `KubernetesMark` on the node/manifest-policy findings | `feat/frontend/register-k8sscan-node` | `documentation/09-ui-ux-design-system.md` §4.5 |

**Done when:** `k8sscan` findings appear in a real scan against a fixture repo with a planted manifest issue.

---

## Phase 10 — `cicdscan`

*`BUILD_GUIDE.md` Phase 10*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-59 | MR | Sep 8 | GitHub Actions workflow rule checks + AI semantic review pass (uses `modules/ai` from Phase 4) — true-positive/near-miss test per rule + a planted prompt-injection fixture workflow proving `prompt_injection_attempt` fires end to end, fixture planted same PR | `feat/cicdscan/workflow-rules-and-ai-pass` | `documentation/05-module-specifications.md` §10; `BUILD_GUIDE.md` Phase 10 |
| SCRUM-60 | MH | Sep 8 | Frontend: register `cicdscan`'s node (reuses `GitHubMark`, no new icon) + confirm `AiPanel`'s "AI-generated" chip renders correctly on a `cicdscan` finding in the Phase 6 raw findings list | `feat/frontend/register-cicdscan-node-and-ai-chip` | `documentation/09-ui-ux-design-system.md` §4.2 |

**Done when:** `cicdscan` findings appear in a real scan against a fixture repo with a planted workflow issue, including at least one AI-reviewed finding.

---

## Phase 11 — `docreview`

*`BUILD_GUIDE.md` Phase 11*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-61 | IH | Sep 10 | AI review of design/requirement documents (uses `modules/ai`): review categories + prompt design — true-positive/near-miss table (document any deliberate deviation, "AI review" rules don't near-miss the same way pattern rules do), fixture planted same PR | `feat/docreview/ai-review-categories-and-prompts` | `documentation/05-module-specifications.md` §11 (or nearest `docreview` section) |
| SCRUM-62 | MR | Sep 10 | Frontend: register `docreview`'s node in `SupplyChainPipeline` + `GeminiMark` (this node is where the AI mark is most load-bearing — AI review is the entire output here, not an enrichment) | `feat/frontend/register-docreview-node` | `documentation/09-ui-ux-design-system.md` §4.5 |

**Done when:** `docreview` findings appear in a real scan against a fixture repo containing a planted design-doc issue.

---

## Phase 12 — `pentest`

*`BUILD_GUIDE.md` Phase 12*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-63 | MR | Sep 11 | `internal/scripts/pentest` sandboxed bash suite (`go:embed`) + target validation (reject private/loopback/metadata addresses) + mandatory ownership-attestation gate — true-positive/near-miss test per rule against a controlled, owned throwaway target only, never a third party | `feat/pentest/sandboxed-bash-suite-and-attestation-gate` | `documentation/05-module-specifications.md` §12; `documentation/12-security-and-threat-model.md` |
| SCRUM-64 | MH | Sep 11 | Frontend: full New Scan wizard (Screen 5) with the mandatory attestation step (full authorisation text, explicit checkbox, Start disabled until checked, NFR-CMP-001) + register `pentest`'s node (branches directly off scan start, not workspace prep) — no brand mark | `feat/frontend/new-scan-wizard-with-attestation` | `documentation/09-ui-ux-design-system.md` §5.5, §4.8 |

**Done when:** all seven engines are registered, a full supply-chain scan runs all of them, and the live graph shows all seven nodes animating, converging into AI enrichment.

---

## Phase 13 — Scoring and reporting

*`BUILD_GUIDE.md` Phase 13*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-65 | MR | Sep 15 | Migrations `00007` remainder (`finding_status_history`), `00008` (`ai_suggestions`), `00009` remainder (`risk_assessments`); `modules/scoring` (weighted risk formula, gate verdict, saturation/critical/secret floors — pure, exhaustively unit-tested); `modules/reporting` (query/filter/sort/pagination, triage, fingerprint correlation, `GET /scans/{id}/export?format=json`, `pdf`/`sarif` return `501`); AI enrichment wiring within per-scan token budget | `feat/scoring/scoring-reporting-and-ai-enrichment` | `documentation/11-risk-scoring-and-severity.md`; `documentation/10-ai-integration.md` §8 |
| SCRUM-66 | GH | Sep 15 | Frontend: `FindingsTable` explorer (filter/sort/pagination/search/triage, replaces the raw list) + finding detail view (`CodeBlock`, `CvssChip`, `CweChip`/`CveChip`, AI explanation + `PatchDiff`) + `AiPanel` `unavailable`/`budget-exhausted` states + `PartialResultBanner` at dashboard level + JSON export button (PDF/SARIF shown as "coming soon") + `RiskGauge`/trend/per-engine breakdown (drops the Phase 2 "Preview — sample data" badge) + first real `MetricCard` placement | `feat/frontend/findings-table-and-scan-report` | `documentation/09-ui-ux-design-system.md` §4.2 |

**Done when:** a completed scan shows a single 0–100 score with a per-engine breakdown, browsable end to end from the dashboard down to an individual finding's AI explanation and patch. (`GET /dashboard/overview` org-wide rollup is explicitly out of scope for this build — no assigned screen number, see `BUILD_GUIDE.md`'s gap-check note.)

---

## Phase 14 — Hardening and demo readiness

*`BUILD_GUIDE.md` Phase 14 — cross-cutting polish/CI/demo prep, no new frontend catch-up phase (frontend and backend grew up together phase by phase)*

| Ticket | Owner | Due | Task | Branch | Docs |
|---|---|---|---|---|---|
| SCRUM-67 | MR | Sep 18 | Accessibility pass on the accumulated UI: automated `axe-core` checks + manual keyboard/screen-reader/colour-blind checklist | `chore/frontend/accessibility-pass` | `documentation/15-testing-strategy.md` §8 |
| SCRUM-68 | MR | Sep 18 | Build out `testdata/fixtures/{fixture-vulnerable,fixture-clean,fixture-typical,fixture-noisy,fixture-one-secret}` with `EXPECTED.yaml` catalogues + golden CI gates (≥90% detection on `fixture-vulnerable`, zero non-informational findings on `fixture-clean`) | `test/fixtures/golden-fixture-repositories` | `documentation/15-testing-strategy.md` §5 |
| SCRUM-69 | MR | Sep 18 | Self-scan CI job (GuardPipe scans its own repo, fails on any CRITICAL) + `container-scan` (Trivy) + `dependency-scan` (`govulncheck` + `npm audit`) CI jobs | `chore/ci/self-scan-and-supply-chain-jobs` | `documentation/13-devops-and-environments.md` §8 |
| SCRUM-70 | MR | Sep 18 | Demo prep: `make seed`, pre-warm the AI cache, run the full UAT checklist | `chore/demo/seed-cache-and-uat-checklist` | `documentation/15-testing-strategy.md` §8 |
| SCRUM-71 | MR | Sep 18 | Tag `v1.0.0` | — (tag only, no branch) | — |

**Done when:** every box in `documentation/01-project-charter.md` §10 ("Success criteria") is checked.

---

## Progress tracker (mirror this against the Jira board)

- [x] Phase 4 — SCRUM-44
- [ ] Phase 5 — SCRUM-45, 46, 47
- [ ] Phase 6 — SCRUM-48, 49, 50, 51, 52
- [ ] Phase 7 — SCRUM-53, 54
- [ ] Phase 8 — SCRUM-55, 56
- [ ] Phase 9 — SCRUM-57, 58
- [ ] Phase 10 — SCRUM-59, 60
- [ ] Phase 11 — SCRUM-61, 62
- [ ] Phase 12 — SCRUM-63, 64
- [ ] Phase 13 — SCRUM-65, 66
- [ ] Phase 14 — SCRUM-67, 68, 69, 70, 71
