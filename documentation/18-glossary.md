# 18 — Glossary

| Field | Value |
|---|---|
| **Document** | Glossary |
| **Project** | GuardPipe |
| **Version** | 1.0 |
| **Status** | Draft |
| **Last updated** | 2026-07-29 |

---

## GuardPipe terms

| Term | Definition |
|---|---|
| **Engine** | One of seven analysis modules (`docreview`, `codescan`, `depscan`, `containerscan`, `k8sscan`, `cicdscan`, `pentest`). All implement the same `Engine` interface |
| **Finding** | A single normalised security issue. Every engine produces this one type, which is what makes one dashboard and one score possible |
| **Fingerprint** | SHA-256 of rule ID + normalised location + normalised evidence. Identifies the same issue across scans. Deliberately excludes line numbers |
| **Rule** | A deterministic detector inside an engine. Has a permanent namespaced ID such as `codescan.injection.sql-string-concat` |
| **Core / Stretch** | Rule and requirement tiers. Core must ship; Stretch is cut without discussion if the schedule tightens |
| **Scan** | One execution of one or more engines against a target |
| **Scan job** | One engine's execution within a scan. Has its own status; a failed job does not fail the scan |
| **GuardPipe Risk Score** | Unified 0–100 value, higher is worse. Computed deterministically from findings |
| **Verdict** | `pass` / `warn` / `block` — the release-gate outcome derived from the score |
| **Critical floor** | A minimum score imposed when critical findings exist, so one catastrophic issue cannot be averaged away |
| **Saturation** | The scoring property where each additional finding of a severity class contributes less than the last |
| **Attestation** | The recorded declaration that a user owns or is authorised to test a pentest target. Legally significant; required before any active testing |
| **Sandbox** | An ephemeral, resource-capped, non-root, network-restricted container in which all shell execution occurs |
| **Golden fixture** | A deliberately vulnerable (or deliberately clean) test repository with a catalogue of exactly what should be found |
| **Near-miss test** | A test asserting a rule does **not** fire on similar-but-safe code. The false-positive control |
| **Vertical slice** | One feature built end-to-end through every layer, early, to prove the path works — here, `depscan` in Sprint 0 |
| **Partial scan** | A scan that completed with one or more engines failed. Never reported as `pass` |
| **Supply chain pipeline** | The seven-stage visual on the dashboard, each stage coloured by its worst finding severity |

---

## Security terms

| Term | Definition |
|---|---|
| **SBOM** | Software Bill of Materials — a complete inventory of components in a software artifact |
| **CVE** | Common Vulnerabilities and Exposures — a unique identifier for a publicly disclosed vulnerability, e.g. `CVE-2021-44228` |
| **CWE** | Common Weakness Enumeration — a taxonomy of *classes* of weakness, e.g. `CWE-89` (SQL Injection). A CVE is an instance; a CWE is a category |
| **CVSS** | Common Vulnerability Scoring System — a 0.0–10.0 severity score with a vector string describing exploitability and impact |
| **GHSA** | GitHub Security Advisory identifier |
| **OSV** | Open Source Vulnerabilities — a distributed vulnerability database and schema; our advisory source |
| **NVD** | National Vulnerability Database (NIST) |
| **EPSS** | Exploit Prediction Scoring System — probability a vulnerability will be exploited. Named as future work |
| **KEV** | CISA's Known Exploited Vulnerabilities catalogue |
| **SAST** | Static Application Security Testing — analysing source code without executing it |
| **DAST** | Dynamic Application Security Testing — testing a running application. Our `pentest` engine is a limited form |
| **SCA** | Software Composition Analysis — dependency vulnerability scanning |
| **IaC** | Infrastructure as Code — Kubernetes manifests, Terraform, Helm |
| **Supply chain attack** | Compromising software by attacking its dependencies, build system, or distribution rather than the software itself |
| **Typosquatting** | Publishing a malicious package with a name close to a popular one |
| **Taint analysis** | Tracking untrusted data from a *source* through *propagators* to a dangerous *sink*, accounting for *sanitisers* |
| **Source / Sink / Sanitiser** | Where untrusted data enters / where it becomes dangerous / what makes it safe |
| **False positive** | A reported issue that is not real. Destroys user trust faster than a miss |
| **False negative** | A real issue that was not reported. Invisible, and the failure mode that matters most for a scanner |
| **SARIF** | Static Analysis Results Interchange Format — the standard JSON format for static analysis output |
| **Prompt injection** | Embedding instructions in content processed by an LLM, attempting to override the system's intent |
| **SSRF** | Server-Side Request Forgery — tricking a server into making requests to attacker-chosen destinations, often internal |
| **XSS** | Cross-Site Scripting — injecting script into a page viewed by others |
| **ReDoS** | Regular expression Denial of Service via catastrophic backtracking. Structurally impossible in Go, which uses RE2 |
| **Zip bomb** | A small archive that expands to an enormous size, exhausting memory or disk |
| **DNS rebinding** | Changing a hostname's resolution between validation and use, to bypass address restrictions |
| **Defence in depth** | Multiple independent controls, so one failure is not a breach |
| **Least privilege** | Granting only the minimum permissions required |
| **STRIDE** | Threat modelling taxonomy: Spoofing, Tampering, Repudiation, Information disclosure, Denial of service, Elevation of privilege |
| **OWASP Top 10** | The ten most critical web application security risks, published by OWASP |
| **ASVS** | OWASP Application Security Verification Standard |
| **CIS Benchmark** | Center for Internet Security configuration hardening guidelines |
| **NIST SSDF** | NIST Secure Software Development Framework (SP 800-218) |

---

## Kubernetes terms

| Term | Definition |
|---|---|
| **RBAC** | Role-Based Access Control — Kubernetes' permission model |
| **Role / ClusterRole** | A set of permissions, namespaced / cluster-wide |
| **RoleBinding / ClusterRoleBinding** | Grants a Role to a subject |
| **ServiceAccount** | The identity a pod uses to talk to the API server |
| **`cluster-admin`** | The built-in superuser ClusterRole. Binding to it is effectively full cluster compromise |
| **Privilege escalation path** | A chain of permissions leading to greater access than intended — e.g. `create pods` allows scheduling a pod that mounts a privileged ServiceAccount token |
| **Privileged container** | A container with `securityContext.privileged: true` — nearly equivalent to root on the node |
| **`hostPath`** | A volume mounting a host directory into a pod. Mounting `/` or the Docker socket is a container escape |
| **`hostNetwork` / `hostPID` / `hostIPC`** | Sharing the node's network, process, or IPC namespace |
| **Capabilities** | Fine-grained Linux root privileges. `SYS_ADMIN` is close to full root |
| **PSA / PSS** | Pod Security Admission enforcing Pod Security Standards: `privileged`, `baseline`, `restricted` |
| **NetworkPolicy** | Firewall rules for pod-to-pod traffic. Without one, all pods can reach all pods |
| **Admission control** | Cluster-side validation or mutation of resources before they are persisted |

---

## Container terms

| Term | Definition |
|---|---|
| **Image** | An immutable filesystem template plus configuration |
| **Layer** | One filesystem change set. Images are stacks of layers; **a deleted file remains in the layer below** |
| **Digest** | Content hash uniquely identifying an image, e.g. `sha256:ab12…`. Immutable, unlike a tag |
| **Tag** | A mutable human label such as `:latest` or `:v1.2`. Can be repointed at any time |
| **Multi-stage build** | Using one stage to build and a separate minimal stage to run, keeping toolchains out of the final image |
| **Distroless** | A base image with no shell and no package manager — minimal attack surface |
| **Rootless Docker** | Running the daemon as an unprivileged user; the remediation for the socket-mount risk |
| **Container escape** | Breaking out of a container to the host |

---

## CI/CD terms

| Term | Definition |
|---|---|
| **GitHub Actions** | GitHub's CI/CD platform |
| **Workflow / Job / Step** | A YAML pipeline definition / a unit running on one runner / a single command or action |
| **Action** | A reusable workflow component, referenced as `owner/repo@ref` |
| **Pinning to SHA** | Referencing an action by full commit hash rather than a mutable tag. A tag can be repointed at malicious code |
| **`pull_request_target`** | A trigger running with the base repository's secrets and a write token. Combined with checking out PR code, it is one of the most reliably exploited CI patterns in the wild |
| **Script injection** | Interpolating `${{ github.event.* }}` values into a `run:` block, letting attacker-controlled text execute as shell |
| **`GITHUB_TOKEN`** | The automatically-provided token for a workflow run. Should be scoped with an explicit `permissions:` block |
| **Self-hosted runner** | A runner on your own infrastructure. Dangerous on public repositories with fork triggers |
| **GitOps** | Git as the single source of truth for infrastructure state |

---

## Engineering terms

| Term | Definition |
|---|---|
| **Modular monolith** | One deployable unit with strictly enforced internal module boundaries |
| **ADR** | Architecture Decision Record — one significant decision, its context, options, and consequences |
| **C4 model** | Architecture diagramming at four levels: Context, Container, Component, Code |
| **arc42** | A template for architecture documentation |
| **SRS** | Software Requirements Specification |
| **Traceability matrix** | Mapping requirements to the modules, endpoints, owners, and tests that satisfy them |
| **RACI** | Responsible, Accountable, Consulted, Informed |
| **WBS** | Work Breakdown Structure |
| **Definition of Done** | The explicit checklist a piece of work must satisfy to be considered complete |
| **12-factor** | An application design methodology; here mainly "configuration comes from the environment" |
| **Idempotent** | Producing the same result whether run once or many times |
| **At-least-once delivery** | A queue guarantee that a message is delivered one or more times — requires idempotent consumers |
| **Circuit breaker** | Stopping calls to a failing dependency for a period, rather than retrying into a wall |
| **Single-flight** | Collapsing concurrent identical requests into one |
| **Optimistic update** | Applying a UI change immediately and rolling back if the server rejects it |
| **Virtualisation (UI)** | Rendering only visible rows of a long list |
| **Conventional Commits** | A commit message format: `type(scope): subject` |
| **Semantic Versioning** | `MAJOR.MINOR.PATCH` with defined change semantics |
| **RFC 9457** | Problem Details for HTTP APIs — the standard error response format |
| **WCAG** | Web Content Accessibility Guidelines. We target 2.1 Level AA |

---

## Abbreviations

| | |
|---|---|
| **AES-GCM** | Advanced Encryption Standard, Galois/Counter Mode |
| **API** | Application Programming Interface |
| **CORS** | Cross-Origin Resource Sharing |
| **CSP** | Content Security Policy |
| **DTO** | Data Transfer Object |
| **ERD** | Entity Relationship Diagram |
| **HSTS** | HTTP Strict Transport Security |
| **JWT** | JSON Web Token |
| **LLM** | Large Language Model |
| **MSW** | Mock Service Worker |
| **PAT** | Personal Access Token |
| **RBAC** | Role-Based Access Control |
| **REST** | Representational State Transfer |
| **SPA** | Single Page Application |
| **SSE** | Server-Sent Events |
| **TTL** | Time To Live |
| **UAT** | User Acceptance Testing |
| **UUID** | Universally Unique Identifier |
| **WIP** | Work In Progress |
