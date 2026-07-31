# 20 — Demo Repositories

| Field | Value |
|---|---|
| **Document** | Demo Repositories |
| **Project** | GuardPipe |
| **Version** | 1.0 |
| **Status** | Draft |
| **Owner** | Team Lead |
| **Last updated** | 2026-07-31 |

### Revision history

| Version | Date | Author | Change |
|---|---|---|---|
| 1.0 | 2026-07-31 | Team | Initial plan for the two live demo repositories |

---

## 1. Purpose

Beyond the golden fixture repositories used for automated CI scoring (§5 of [15 — Testing Strategy](15-testing-strategy.md)), the team maintains two **real, standalone GitHub repositories** that GuardPipe attaches to a project and scans live. These exist for two things the local fixtures can't do:

1. **Manual/exploratory testing** of the actual product path — clone via `modules/vcs`, run a real scan, watch real findings land — not just `go test` against a local directory.
2. **The teacher-facing demo** — a clear before/after story. Point GuardPipe at the vulnerable repo, get a low score and a `block` verdict with a wall of findings; point it at the clean repo, get a high score and a `pass` verdict. That contrast is the single clearest way to show the platform working end to end.

## 2. Relationship to the golden fixtures — these are not the same thing

| | Golden fixtures (`testdata/fixtures/*`) | Demo repositories (this doc) |
|---|---|---|
| Location | Inside the GuardPipe repo, `testdata/fixtures/` | Separate, standalone GitHub repositories |
| Consumer | `go test` — engines read the directory directly | The real product — cloned by `modules/vcs` via the GitHub adapter, same as any user's repo |
| Purpose | CI-enforced detection-rate and false-positive-rate scoring (≥ 90% on `fixture-vulnerable`, zero on `fixture-clean`) | Human-facing manual testing and live demo |
| Catalogued? | Yes — `EXPECTED.yaml` per fixture, checked in CI | No formal catalogue required; findings are whatever the engines actually surface |
| When it's built | Incrementally, in the same PR as each rule (testing strategy §5, principle: "plant the fixture case in the same PR") | Incrementally too, but content can lag behind — see §5 |

**Do not fold these into one thing.** The golden fixtures must stay deterministic, catalogued, and CI-gated — that's what makes the ≥90% detection-rate number meaningful. The demo repositories are allowed to be looser (real repo shape, README, multiple files that aren't all planted issues) because their job is to look like a real project, not to be a precise scoring instrument.

Where it's convenient, the demo repositories may **reuse the same planted issues** as `fixture-vulnerable` / `fixture-clean` so the team isn't authoring vulnerable code twice — but the demo repos are free to add more realistic surrounding scaffolding (a plausible app, a real `Dockerfile`, real-looking K8s manifests) that the tight, minimal golden fixtures don't need.

## 3. The two repositories

| Repo | Intent | Expected verdict |
|---|---|---|
| `guardpipe-demo-vulnerable` | Deliberately insecure across every SDLC stage GuardPipe scans | Low score, `block` |
| `guardpipe-demo-clean` | The same kind of application, built correctly | High score, `pass` |

**Ownership:** both are real GitHub repositories under the project owner's account (`Ruhanyat-994`), created and owned outside this monorepo — not a subdirectory of `GuardPipe` itself. They get attached to a GuardPipe project the same way any user's repository would be (Phase 3's `POST /projects/{id}/repositories` flow), which also makes them useful as the manual verification step for that phase's "done when" criterion.

**Coverage — `guardpipe-demo-vulnerable` should carry at least one planted issue per engine**, mirroring the categories already defined for `fixture-vulnerable` (testing strategy §5):

| Stage | Engine | Example planted issue |
|---|---|---|
| Design docs | `docreview` | A requirements/design doc with a missing threat-model section, or containing prompt-injection-style text, for the AI reviewer to flag |
| Code | `codescan` | SQL built by string concatenation, reflected XSS, command injection, path traversal, a hardcoded secret, weak crypto (e.g. MD5 for passwords), insecure deserialization |
| Dependencies | `depscan` | A manifest pinned to a package version with a known OSV/CVE advisory |
| Containers | `containerscan` | Dockerfile running as root, no `USER` directive, `latest` tag, secrets baked into a layer |
| Kubernetes | `k8sscan` | Privileged container, `hostPath` mount, missing `NetworkPolicy`, a `ClusterRoleBinding` granting `cluster-admin` |
| CI/CD | `cicdscan` | A GitHub Actions workflow with a `pull_request_target` + checkout-of-fork-code pattern, or a step that echoes a secret |
| Pentest | `pentest` | Only in scope once a live deployment exists and the ownership-attestation gate (§4) is satisfied — see the caution in §6 |

`guardpipe-demo-clean` is the same application shape, written the secure way, so the two repos read as "before/after" rather than as unrelated projects.

## 4. Naming and fake data

- Any "secret" planted in `guardpipe-demo-vulnerable` (API key, password, token) **must be a fake, non-functional value** — never a real credential, even a revoked one, since revoked secrets still trigger the same class of "don't do this" lesson without any residual risk if leaked further.
- Keep obviously-fake values recognisable as fake in the commit itself (e.g. an AWS-shaped key that isn't a real AWS key) — the goal is "a scanner should flag this," not "let's see if anyone can actually use it."

## 5. When content actually gets built

The repos need to **exist** starting in Phase 3, so they can be attached to a project and prove the GitHub-attachment flow works end to end — that part doesn't depend on any engine being built yet, since attaching a repo is just metadata + a clone, no scanning happens.

Full content, however, tracks the engine build-out in [`BUILD_GUIDE.md`](../BUILD_GUIDE.md) Phase 7: there's no point planting a Kubernetes manifest issue before `k8sscan` exists to find it. Practical order:

1. **Phase 3** — both repos exist on GitHub with a minimal, real-looking scaffold (a small app, a `Dockerfile`, a basic k8s manifest, a CI workflow) so they can be attached as projects.
2. **Phase 6** (`depscan`, the first engine) — plant the dependency-related issues so the first end-to-end scan has something real to find.
3. **Phase 7** (remaining six engines) — plant each engine's issue in the demo repo in the same window the engine's rule lands, same discipline as the golden fixtures ("plant the fixture case in the same PR").
4. **Phase 9** (hardening/demo prep) — final pass to confirm the vulnerable repo produces a `block` verdict and the clean repo a `pass` verdict, since that contrast is the actual demo.

## 6. Caution — the pentest engine

`pentest` only runs against a target that passes the ownership-attestation gate in [12 — Security & Threat Model](12-security-and-threat-model.md) (DNS-resolve, reject RFC1918/loopback/metadata addresses, explicit ownership attestation). If `guardpipe-demo-vulnerable` is ever actually deployed somewhere for the pentest engine to probe, that deployment is owned by the team and the attestation step must be honoured for real — this repo does not get an exemption from that gate just because it's a demo fixture. Until Phase 7's `pentest` engine exists, this repo has no live deployment and the point is moot.

## 7. Cross-references

- [15 — Testing Strategy](15-testing-strategy.md) §5 — the golden fixtures these demo repos are deliberately *not* replacing.
- [12 — Security & Threat Model](12-security-and-threat-model.md) — pentest target validation and ownership attestation.
- [`BUILD_GUIDE.md`](../BUILD_GUIDE.md) — Phase 3 (repos must exist and be attachable), Phase 6/7 (content lands as engines are built), Phase 9 (final verdict check).
