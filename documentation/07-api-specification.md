# 07 — API Specification

| Field | Value |
|---|---|
| **Document** | API Specification |
| **Project** | GuardPipe |
| **Version** | 1.0 |
| **Status** | Draft |
| **Style** | REST · JSON · OpenAPI 3.1 conventions · RFC 9457 errors |
| **Base URL** | `http://localhost:8080/api/v1` |
| **Authors** | GuardPipe Team |
| **Last updated** | 2026-07-29 |

### Revision history

| Version | Date | Author | Change |
|---|---|---|---|
| 1.0 | 2026-07-29 | Team | Initial API contract |

> **Change control:** this is the frontend/backend contract. Breaking changes require **two approvals** and a note to the frontend owner. Freeze target: end of Sprint 0.

---

## 1. Conventions

| Aspect | Rule |
|---|---|
| Base path | `/api/v1` — the version is in the path, always |
| Format | JSON only; `Content-Type: application/json; charset=utf-8` |
| Naming | `snake_case` for all JSON fields — consistent with the database, no translation layer confusion |
| IDs | UUIDv4 strings |
| Timestamps | RFC 3339 UTC with `Z` suffix: `2026-08-14T09:31:07Z` |
| Enums | lowercase snake_case strings, never integers |
| Nulls | Present with `null`, not omitted — the frontend can rely on key presence |
| Empty collections | `[]`, never `null` |
| Auth | `Authorization: Bearer <access_token>` |
| Idempotency | `POST` scan creation accepts an optional `Idempotency-Key` header |
| Correlation | Every response carries `X-Request-ID` |

### 1.1 HTTP method semantics

| Method | Use | Idempotent |
|---|---|---|
| `GET` | Read | yes |
| `POST` | Create, or trigger an action | no |
| `PATCH` | Partial update | yes |
| `DELETE` | Remove / archive | yes |

`PUT` is not used — every update in this API is partial.

### 1.2 Status codes

| Code | When |
|---|---|
| 200 | Successful read or update |
| 201 | Resource created (with `Location` header) |
| 202 | Accepted for asynchronous processing (scan creation) |
| 204 | Success, no body (logout, delete) |
| 400 | Malformed request or validation failure |
| 401 | Missing, invalid, or expired access token |
| 403 | Authenticated but role-forbidden |
| 404 | Not found **or not owned by the caller** (see §1.4) |
| 409 | State conflict (scan already running, duplicate name) |
| 422 | Semantically invalid (target fails validation) |
| 429 | Rate limited; includes `Retry-After` |
| 500 | Unexpected server error |
| 502 | Upstream dependency failed (Gemini, OSV, GitHub) |
| 503 | Not ready (database unavailable) |
| 504 | Upstream timeout |

### 1.3 Error format — RFC 9457

```json
{
  "type": "https://guardpipe.dev/errors/validation-failed",
  "title": "Validation failed",
  "status": 400,
  "detail": "One or more fields are invalid.",
  "instance": "/api/v1/projects",
  "code": "project.validation_failed",
  "request_id": "01J8XZ4K9P2M3N4Q5R6S7T8U9V",
  "errors": [
    { "field": "name", "message": "must be between 1 and 120 characters" },
    { "field": "repository_url", "message": "must be a valid HTTPS GitHub URL" }
  ]
}
```

| Field | Notes |
|---|---|
| `code` | Machine-readable, stable, namespaced. **This is what the frontend switches on** — never parse `title` or `detail` |
| `detail` | Safe for display to a user. Never contains stack traces, SQL, or internal paths |
| `errors` | Present only on validation failures |
| `request_id` | Matches the log entry — this is how a user-reported bug gets traced |

**Error code catalogue (partial)**

| Code | Status | Meaning |
|---|---|---|
| `auth.invalid_credentials` | 401 | Wrong email or password |
| `auth.token_expired` | 401 | Access token expired — refresh and retry |
| `auth.token_invalid` | 401 | Malformed or tampered token |
| `auth.refresh_reused` | 401 | Refresh token replay detected; family revoked |
| `auth.forbidden_role` | 403 | Role lacks permission |
| `project.not_found` | 404 | Not found or not owned |
| `project.name_taken` | 409 | Duplicate name in the organisation |
| `project.credential_required` | 422 | Private repository with no credential |
| `target.blocked_address` | 422 | Resolves to a blocked range |
| `target.not_attested` | 409 | Pentest attempted without attestation |
| `target.dns_rebinding_suspected` | 422 | Resolution changed since validation |
| `scan.already_running` | 409 | A scan is already in progress for this project |
| `scan.no_engines_selected` | 400 | Empty engine list |
| `scan.repository_unreachable` | 422 | Clone failed |
| `scan.repository_too_large` | 422 | Exceeds the size limit |
| `finding.reason_required` | 400 | Suppression justification too short |
| `ai.unavailable` | 502 | Gemini unreachable |
| `ai.budget_exhausted` | 200* | Not an error — returned as a field, not a failure |
| `ratelimit.exceeded` | 429 | Too many requests |

### 1.4 Authorisation leak policy

A resource that exists but belongs to another organisation returns **404, not 403** (FR-IAM-008). 403 confirms the resource exists, which is an information leak. 403 is used only for *role* failures on resources the caller can otherwise see.

### 1.5 Pagination, filtering, sorting

```
GET /api/v1/scans/{scan_id}/findings
      ?page=2&page_size=25
      &severity=critical,high
      &engine=codescan
      &status=open
      &cwe=CWE-89
      &file_path=src/
      &q=injection
      &sort=-severity
```

| Parameter | Rule |
|---|---|
| `page` | 1-based, default 1 |
| `page_size` | default 25, max 100 (FR-RPT-002) |
| Multi-value filters | comma-separated, OR within a field, AND across fields |
| `q` | full-text over title + description |
| `sort` | field name, `-` prefix for descending; whitelist only |

**Collection envelope**

```json
{
  "data": [ { "…": "…" } ],
  "pagination": { "page": 2, "page_size": 25, "total": 137, "total_pages": 6 }
}
```

Single resources are returned bare, with no `data` wrapper. Two shapes, consistently applied.

### 1.6 Rate limits

| Scope | Limit | Key |
|---|---|---|
| `POST /auth/login`, `/auth/register` | 5 / min | IP |
| `POST .../scans` | 10 / hour | user |
| All other authenticated | 100 / min | user |
| Unauthenticated | 20 / min | IP |

Responses carry `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`; 429 adds `Retry-After`.

---

## 2. Authentication

### Token lifecycle

```mermaid
sequenceDiagram
    participant C as SPA
    participant A as API
    C->>A: POST /auth/login {email, password}
    A-->>C: 200 {access_token, expires_in: 900, user}<br/>Set-Cookie: gp_refresh=…; HttpOnly; Secure; SameSite=Strict
    Note over C: access token held in memory only —<br/>never localStorage (XSS exfiltration)
    C->>A: GET /projects (Bearer access_token)
    A-->>C: 200
    Note over C: 15 minutes pass
    C->>A: GET /projects
    A-->>C: 401 {code: "auth.token_expired"}
    C->>A: POST /auth/refresh (cookie sent automatically)
    A-->>C: 200 {access_token}<br/>Set-Cookie: gp_refresh=<rotated>
    C->>A: GET /projects (retry, new token)
    A-->>C: 200
```

**Why the refresh token is a cookie and the access token is not:** the refresh token is long-lived and must survive a page reload, so it needs `HttpOnly` protection from XSS. The access token is short-lived and held in JavaScript memory, so a reload simply triggers a silent refresh. This is the standard SPA compromise and is documented so nobody "improves" it into `localStorage`.

### Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/auth/register` | none | Create a user |
| `POST` | `/auth/login` | none | Authenticate |
| `POST` | `/auth/refresh` | cookie | Rotate tokens |
| `POST` | `/auth/logout` | Bearer | Revoke the refresh token |
| `GET` | `/auth/me` | Bearer | Current user |

**`POST /auth/register`**
```json
// request
{ "email": "nadia@example.com", "display_name": "Nadia R.", "password": "correct-horse-battery" }
// 201
{ "id": "…", "email": "nadia@example.com", "display_name": "Nadia R.",
  "role": "member", "created_at": "2026-08-05T10:00:00Z" }
```

**`POST /auth/login`**
```json
// 200
{ "access_token": "eyJhbGciOi…", "token_type": "Bearer", "expires_in": 900,
  "user": { "id": "…", "email": "…", "display_name": "…", "role": "member" } }
```
Failure is always `401 auth.invalid_credentials` with an identical message for unknown user and wrong password — no user enumeration.

---

## 3. Projects

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/projects` | viewer | List projects |
| `POST` | `/projects` | member | Create |
| `GET` | `/projects/{id}` | viewer | Detail with latest scan summary |
| `PATCH` | `/projects/{id}` | member | Update name/description |
| `DELETE` | `/projects/{id}` | admin | Archive (soft) |
| `POST` | `/projects/{id}/repository` | member | Attach/replace repository |
| `PUT` | `/projects/{id}/credential` | member | Set the GitHub PAT |
| `DELETE` | `/projects/{id}/credential` | member | Remove the PAT |

**`POST /projects`**
```json
// request
{ "name": "Payments API", "description": "Core payment service",
  "repository_url": "https://github.com/acme/payments-api" }
// 201
{
  "id": "9f1c…", "name": "Payments API", "description": "Core payment service",
  "status": "active",
  "repository": {
    "id": "3a7e…", "provider": "github", "url": "https://github.com/acme/payments-api",
    "owner": "acme", "name": "payments-api", "default_branch": "main",
    "is_private": false, "size_kb": 18432
  },
  "has_credential": false,
  "latest_scan": null,
  "created_at": "2026-08-05T10:04:00Z"
}
```

**`PUT /projects/{id}/credential`**
```json
// request
{ "kind": "github_pat", "token": "ghp_xxxxxxxxxxxxxxxxxxxx" }
// 200 — the token is NEVER echoed back (FR-PRJ-004)
{ "has_credential": true, "hint": "ghp_••••xxxx", "updated_at": "2026-08-05T10:06:00Z" }
```

---

## 4. Pentest targets

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/projects/{id}/targets` | viewer | List targets |
| `POST` | `/projects/{id}/targets` | member | Register a target (validates + resolves) |
| `POST` | `/targets/{id}/attest` | member | Accept the authorisation attestation |
| `DELETE` | `/targets/{id}` | member | Revoke |

**`POST /projects/{id}/targets`**
```json
// request
{ "target": "https://staging.acme.example" }
// 201
{ "id": "b2c4…", "target": "https://staging.acme.example",
  "normalized_host": "staging.acme.example",
  "pinned_ips": ["203.0.113.10"],
  "status": "awaiting_attestation",
  "last_resolved_at": "2026-08-05T10:10:00Z" }

// 422 when blocked
{ "type": "…/blocked-address", "title": "Target address is blocked", "status": 422,
  "code": "target.blocked_address",
  "detail": "staging.acme.example resolves to 192.168.1.50, which is in a blocked private range." }
```

**`POST /targets/{id}/attest`**
```json
// request
{ "attestation_text_version": "v1",
  "accepted": true,
  "statement": "I confirm I own or am explicitly authorised to test this target." }
// 200
{ "id": "b2c4…", "status": "attested", "attested_at": "2026-08-05T10:11:00Z",
  "attested_by": { "id": "…", "display_name": "Nadia R." } }
```

Attempting a pentest against a target with `status != "attested"` returns `409 target.not_attested`. This is a hard gate, not a warning (NFR-CMP-001).

---

## 5. Scans

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/projects/{id}/scans` | member | Start a scan |
| `GET` | `/projects/{id}/scans` | viewer | Scan history |
| `GET` | `/scans/{id}` | viewer | Scan detail + per-engine job status |
| `POST` | `/scans/{id}/cancel` | member | Cancel |
| `GET` | `/scans/{id}/progress` | viewer | Lightweight progress (polling target) |
| `GET` | `/scans/{id}/events` | viewer | SSE progress stream (**Stretch**) |
| `GET` | `/scans/{id}/export` | viewer | Export report |

**`POST /projects/{id}/scans`**
```json
// request — full supply chain
{ "type": "full_supply_chain", "branch": "main" }

// request — selected engines only (FR-ORC-002)
{ "type": "partial", "engines": ["codescan", "depscan"], "branch": "feature/login" }

// request — standalone pentest (FR-PEN-013)
{ "type": "pentest_only", "target_id": "b2c4…" }

// 202 Accepted, Location: /api/v1/scans/7d3f…
{
  "id": "7d3f…", "project_id": "9f1c…", "type": "full_supply_chain",
  "status": "queued",
  "requested_engines": ["docreview","codescan","depscan","containerscan","k8sscan","cicdscan"],
  "branch": "main", "commit_sha": null,
  "queued_at": "2026-08-05T10:20:00Z",
  "jobs": [
    { "id": "…", "engine": "codescan", "status": "queued" },
    { "id": "…", "engine": "depscan",  "status": "queued" }
  ]
}
```

**Asynchronous by design.** The API returns 202 immediately; the client polls. A scan takes minutes — holding an HTTP connection open for it would be wrong at every level.

**`GET /scans/{id}`**
```json
{
  "id": "7d3f…", "project_id": "9f1c…", "type": "full_supply_chain",
  "status": "completed",
  "branch": "main", "commit_sha": "a3f9c21…",
  "queued_at": "2026-08-05T10:20:00Z",
  "started_at": "2026-08-05T10:20:03Z",
  "finished_at": "2026-08-05T10:23:41Z",
  "duration_ms": 218000,
  "triggered_by": { "id": "…", "display_name": "Nadia R." },
  "finding_counts": { "critical": 2, "high": 9, "medium": 21, "low": 14, "informational": 7 },
  "risk": {
    "score": 68, "verdict": "block", "previous_score": 74, "delta": -6,
    "is_partial": false,
    "engine_scores": { "codescan": 72, "depscan": 61, "containerscan": 45,
                       "k8sscan": 80, "cicdscan": 55, "docreview": 20 },
    "formula_version": "1.0"
  },
  "jobs": [
    { "id": "…", "engine": "codescan", "status": "succeeded",
      "started_at": "…", "finished_at": "…", "duration_ms": 41000,
      "finding_count": 18,
      "stats": { "files_scanned": 412, "rules_evaluated": 24 },
      "error_reason": null, "skip_reason": null },
    { "id": "…", "engine": "containerscan", "status": "skipped",
      "finding_count": 0, "error_reason": null,
      "skip_reason": "no_target_artifacts" },
    { "id": "…", "engine": "docreview", "status": "failed",
      "finding_count": 0, "error_reason": "ai_unavailable", "skip_reason": null }
  ]
}
```

Note that a `failed` and a `skipped` job both appear in a `completed` scan — this is the contract from FR-ORC-006, and the UI must render all three states distinctly.

**`GET /scans/{id}/progress`** — the polling endpoint, deliberately tiny (2 s interval, FR-UI-002)
```json
{ "scan_id": "7d3f…", "status": "running",
  "progress_pct": 57,
  "engines": [
    { "engine": "codescan",  "status": "succeeded", "progress_pct": 100, "finding_count": 18 },
    { "engine": "depscan",   "status": "running",   "progress_pct": 60,  "finding_count": 7  },
    { "engine": "k8sscan",   "status": "queued",    "progress_pct": 0,   "finding_count": 0  }
  ],
  "updated_at": "2026-08-05T10:22:10Z" }
```
Served from Redis (`gp:progress:{scan_id}`), not PostgreSQL — a 2-second poll must not hit the database.

**`GET /scans/{id}/export?format=json`** → `200` with `Content-Disposition: attachment`. `format=pdf` and `format=sarif` are Stretch and return `501` with `code: "export.format_unsupported"` until implemented.

---

## 6. Findings

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/scans/{id}/findings` | viewer | List with filters (§1.5) |
| `GET` | `/findings/{id}` | viewer | Detail with evidence + AI suggestion |
| `PATCH` | `/findings/{id}/status` | member | Triage |
| `GET` | `/findings/{id}/history` | viewer | Cross-scan history + status transitions |
| `POST` | `/findings/{id}/explain` | member | Request/regenerate AI explanation |

**`GET /scans/{id}/findings` — list item** (deliberately lighter than the detail; the list renders 1,000 rows)
```json
{
  "data": [
    {
      "id": "e5a1…", "engine": "codescan",
      "rule_id": "codescan.injection.sql-string-concat",
      "source": "rule",
      "title": "SQL query built by string concatenation",
      "severity": "high", "confidence": "high", "status": "open",
      "cwe": ["CWE-89"], "cve": [], "owasp": ["A03:2021"],
      "cvss_score": null,
      "location": { "type": "file", "path": "internal/db/user.go",
                    "line_start": 42, "line_end": 44 },
      "first_seen_at": "2026-08-01T09:00:00Z",
      "age_days": 4,
      "has_ai_suggestion": true
    }
  ],
  "pagination": { "page": 1, "page_size": 25, "total": 53, "total_pages": 3 }
}
```

**`GET /findings/{id}` — detail**
```json
{
  "id": "e5a1…", "scan_id": "7d3f…", "engine": "codescan",
  "rule": {
    "id": "codescan.injection.sql-string-concat",
    "title": "SQL query built by string concatenation",
    "category": "injection", "tier": "core",
    "references": ["https://cwe.mitre.org/data/definitions/89.html"]
  },
  "source": "rule",
  "title": "SQL query built by string concatenation",
  "description": "User-controlled input reaches a SQL query that is assembled with string concatenation. An attacker can alter the query structure and read or modify data they should not have access to.",
  "severity": "high", "confidence": "high", "status": "open",
  "cwe": ["CWE-89"], "cve": [], "owasp": ["A03:2021"],
  "cvss_score": null, "cvss_vector": null,
  "location": { "type": "file", "path": "internal/db/user.go",
                "line_start": 42, "line_end": 44, "column": 17 },
  "evidence": [
    { "kind": "code_snippet", "line_start": 39, "line_end": 47,
      "content": "func GetUser(name string) (*User, error) {\n    q := \"SELECT * FROM users WHERE name = '\" + name + \"'\"\n    row := db.QueryRow(q)\n…",
      "content_redacted": false }
  ],
  "remediation": "Use a parameterised query. Pass `name` as a bound argument rather than concatenating it into the SQL string.",
  "ai_suggestion": {
    "explanation": "…",
    "patch_diff": "--- a/internal/db/user.go\n+++ b/internal/db/user.go\n@@ -40,3 +40,3 @@\n-    q := \"SELECT * FROM users WHERE name = '\" + name + \"'\"\n-    row := db.QueryRow(q)\n+    row := db.QueryRow(\"SELECT * FROM users WHERE name = $1\", name)\n",
    "patch_status": "verified",
    "model": "gemini-2.5-flash",
    "generated_at": "2026-08-05T10:23:30Z"
  },
  "history": { "first_seen_scan_id": "…", "first_seen_at": "2026-08-01T09:00:00Z",
               "occurrence_count": 3, "age_days": 4 },
  "created_at": "2026-08-05T10:21:44Z"
}
```

**`PATCH /findings/{id}/status`**
```json
// request
{ "status": "suppressed",
  "reason": "Input is validated by an allowlist in the calling middleware at router.go:88." }
// 200
{ "id": "e5a1…", "status": "suppressed",
  "status_reason": "Input is validated by…",
  "status_changed_by": { "id": "…", "display_name": "Nadia R." },
  "status_changed_at": "2026-08-05T11:00:00Z" }

// 400 when the justification is too short (FR-RPT-005)
{ "code": "finding.reason_required", "status": 400,
  "detail": "A justification of at least 20 characters is required to suppress a finding." }
```

---

## 7. Dashboard

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/dashboard/overview` | viewer | Organisation-wide summary |
| `GET` | `/projects/{id}/dashboard` | viewer | Project summary + supply-chain stage view |
| `GET` | `/projects/{id}/trend` | viewer | Risk-score history |

**`GET /projects/{id}/dashboard`** — drives the main screen, one request
```json
{
  "project": { "id": "9f1c…", "name": "Payments API" },
  "latest_scan": {
    "id": "7d3f…", "status": "completed", "finished_at": "2026-08-05T10:23:41Z",
    "risk": { "score": 68, "verdict": "block", "delta": -6 }
  },
  "severity_breakdown": { "critical": 2, "high": 9, "medium": 21, "low": 14, "informational": 7 },
  "supply_chain_stages": [
    { "stage": "design",     "engine": "docreview",     "status": "failed",    "worst_severity": null,       "finding_count": 0 },
    { "stage": "code",       "engine": "codescan",      "status": "succeeded", "worst_severity": "high",     "finding_count": 18 },
    { "stage": "dependency", "engine": "depscan",       "status": "succeeded", "worst_severity": "critical", "finding_count": 11 },
    { "stage": "build",      "engine": "containerscan", "status": "skipped",   "worst_severity": null,       "finding_count": 0 },
    { "stage": "deploy",     "engine": "k8sscan",       "status": "succeeded", "worst_severity": "critical", "finding_count": 14 },
    { "stage": "pipeline",   "engine": "cicdscan",      "status": "succeeded", "worst_severity": "high",     "finding_count": 6 },
    { "stage": "runtime",    "engine": "pentest",       "status": "not_run",   "worst_severity": null,       "finding_count": 0 }
  ],
  "top_findings": [ { "id": "…", "title": "…", "severity": "critical" } ],
  "recent_scans": [ { "id": "…", "finished_at": "…", "score": 68, "verdict": "block" } ]
}
```

The `supply_chain_stages` array **is** the pipeline visualisation in FR-UI-004 — the backend decides the stage order and mapping so the frontend does not hardcode it.

**`GET /projects/{id}/trend?limit=20`**
```json
{ "points": [
    { "scan_id": "…", "finished_at": "2026-08-01T…", "score": 74, "verdict": "block",
      "counts": { "critical": 3, "high": 11, "medium": 19, "low": 12, "informational": 5 } },
    { "scan_id": "…", "finished_at": "2026-08-05T…", "score": 68, "verdict": "block",
      "counts": { "critical": 2, "high": 9, "medium": 21, "low": 14, "informational": 7 } }
] }
```

---

## 8. Rules catalogue

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/rules` | viewer | All rules, filterable by engine/tier/severity |
| `GET` | `/rules/{id}` | viewer | Rule detail |
| `PATCH` | `/rules/{id}` | admin | Enable/disable a rule |

Lets the UI show "what does GuardPipe actually check?" without the frontend duplicating the rule list — and lets an operator silence a noisy rule without a deploy.

---

## 9. System endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/healthz` | none | Liveness — always 200 if the process is up |
| `GET` | `/readyz` | none | Readiness — 200 only if Postgres + Redis reachable and migrations applied |
| `GET` | `/version` | none | `{version, commit, built_at, go_version}` |
| `POST` | `/webhooks/github` | signature | GitHub push/PR trigger (**Stretch**, FR-ORC-013) |

These sit **outside** `/api/v1` — they are infrastructure, not product API, and must not be versioned with it.

---

## 10. Complete route table

```
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh
POST   /api/v1/auth/logout
GET    /api/v1/auth/me

GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/:id
PATCH  /api/v1/projects/:id
DELETE /api/v1/projects/:id
POST   /api/v1/projects/:id/repository
PUT    /api/v1/projects/:id/credential
DELETE /api/v1/projects/:id/credential

GET    /api/v1/projects/:id/targets
POST   /api/v1/projects/:id/targets
POST   /api/v1/targets/:id/attest
DELETE /api/v1/targets/:id

POST   /api/v1/projects/:id/scans
GET    /api/v1/projects/:id/scans
GET    /api/v1/scans/:id
POST   /api/v1/scans/:id/cancel
GET    /api/v1/scans/:id/progress
GET    /api/v1/scans/:id/events            # Stretch (SSE)
GET    /api/v1/scans/:id/export
GET    /api/v1/scans/:id/findings

GET    /api/v1/findings/:id
PATCH  /api/v1/findings/:id/status
GET    /api/v1/findings/:id/history
POST   /api/v1/findings/:id/explain

GET    /api/v1/dashboard/overview
GET    /api/v1/projects/:id/dashboard
GET    /api/v1/projects/:id/trend

GET    /api/v1/rules
GET    /api/v1/rules/:id
PATCH  /api/v1/rules/:id

GET    /healthz
GET    /readyz
GET    /version
POST   /webhooks/github                    # Stretch
```

**39 endpoints** — 35 under `/api/v1` plus 4 system endpoints. Every one maps to at least one requirement in [02 — SRS](02-srs.md).

---

## 11. Frontend/backend contract rules

| Rule | Why |
|---|---|
| The frontend switches on `code`, never on `detail` or `title` | Messages are copy; codes are contract |
| A new optional response field is **not** a breaking change | Frontend must ignore unknown fields |
| Removing or renaming a field **is** breaking → needs `/api/v2` or a deprecation period | |
| Enum values may be **added**; the frontend must handle unknown values gracefully | New engines and statuses will appear |
| The backend never returns HTML, ever | It is a JSON API |
| No endpoint returns an unbounded collection | Everything paginates |
| Response shapes in this document are the source of truth | If code and doc disagree, one is a bug — file it |

## 12. OpenAPI

The machine-readable `openapi.yaml` is generated in Sprint 1 and lives at `api/openapi.yaml`, served at `/api/v1/openapi.yaml`. **This document remains the human-authored source of truth**; the YAML is derived from it and validated against handler tests in CI.
