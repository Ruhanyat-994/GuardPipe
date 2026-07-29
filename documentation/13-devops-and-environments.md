# 13 — DevOps and Environments

| Field | Value |
|---|---|
| **Document** | DevOps, Environments, and Operations |
| **Project** | GuardPipe |
| **Version** | 1.0 |
| **Status** | Draft |
| **Owner** | Member 6 |
| **Last updated** | 2026-07-29 |

### Revision history

| Version | Date | Author | Change |
|---|---|---|---|
| 1.0 | 2026-07-29 | Team | Initial DevOps design |

---

## 1. Environments

| Environment | Where | Purpose | Data |
|---|---|---|---|
| **Local** | Developer machine, Docker Compose | Development | Seeded fixtures |
| **CI** | GitHub Actions runners | Lint, test, build, self-scan | Ephemeral |
| **Demo** | One designated laptop, Docker Compose | Presentation | Pre-seeded, pre-warmed AI cache |

There is no staging or production environment this semester. That is a scope decision, not an oversight — the charter names local Docker Compose as the acceptance target ([01 §8](01-project-charter.md#8-constraints)). The architecture keeps a cloud path open ([03 §9](03-architecture-overview.md#9-deployment-view)) without spending time on it now.

---

## 2. Prerequisites

| Tool | Version | Check |
|---|---|---|
| Docker Desktop / Engine | 24+ | `docker --version` |
| Docker Compose | v2 | `docker compose version` |
| Go | 1.23+ | `go version` |
| Node.js | 20+ | `node --version` |
| Git | 2.40+ | `git --version` |
| make | any | `make --version` |

Everyone installs these on day one of Sprint 0. A teammate blocked on tooling in week two is a week lost out of four.

---

## 3. Local topology

```mermaid
flowchart TB
    subgraph net["docker network: guardpipe-net"]
        WEB["guardpipe-web<br/>nginx :80 → host :5173"]
        APP["guardpipe<br/>Go :8080 → host :8080"]
        PG[("postgres:16-alpine<br/>:5432<br/>volume: pgdata")]
        RD[("redis:7-alpine<br/>:6379")]
    end
    HOST["host: /var/run/docker.sock"]
    SBX["ephemeral sandbox containers"]
    WEB --> APP
    APP --> PG
    APP --> RD
    APP -. mounted .-> HOST
    HOST -. creates .-> SBX
```

| Service | Image | Ports | Volumes | Health check |
|---|---|---|---|---|
| `guardpipe` | built from `Dockerfile` | 8080 | `docker.sock`, `workspace` | `GET /readyz` |
| `guardpipe-web` | built from `frontend/Dockerfile` | 5173→80 | — | `GET /` |
| `postgres` | `postgres:16-alpine` | 5432 | `pgdata` | `pg_isready` |
| `redis` | `redis:7-alpine` | 6379 | — | `redis-cli ping` |

**Startup ordering** uses Compose `depends_on` with `condition: service_healthy`. The application additionally retries its database connection at boot — `depends_on` guarantees the container is healthy, not that migrations can run, and a race here produces a confusing failure on every fresh clone.

---

## 4. Container images

### 4.1 Backend — multi-stage

| Stage | Base | Purpose |
|---|---|---|
| `builder` | `golang:1.23-alpine` | `go mod download` (cached layer), then `CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=…"` |
| `runtime` | `gcr.io/distroless/static-debian12:nonroot` | Copy the single static binary + migrations + embedded scripts |

Result: ~25 MB image, non-root, no shell, no package manager. A distroless runtime means a container-escape attempt has essentially nothing to work with — and it also means our own image passes our own `containerscan` rules, which is the point.

### 4.2 Frontend — multi-stage

| Stage | Base | Purpose |
|---|---|---|
| `builder` | `node:20-alpine` | `npm ci` then `npm run build` |
| `runtime` | `nginx:1.27-alpine` | Serve `dist/`, SPA fallback to `index.html`, gzip, security headers |

### 4.3 Sandbox runner image

Purpose-built, pinned by digest, containing only what the pentest scripts need: `bash`, `curl`, `openssl`, `dig`, `nc`, `jq`. No package manager at runtime, no compiler, no Docker client.

**Rules for all images**
- Base images pinned by digest, not tag ([05 §8](05-module-specifications.md#8-containerscan--dockerfile-and-image-analysis) — our own rule)
- Non-root user
- No secrets in any layer
- `.dockerignore` excludes `.git`, `node_modules`, `.env`, test fixtures
- Multi-stage always

---

## 5. Configuration reference

All configuration is environment variables (NFR-PRT-002). No config files, no runtime mutation.

### 5.1 Core

| Variable | Default | Required | Description |
|---|---|---|---|
| `GUARDPIPE_ENV` | `development` | no | `development` \| `production` |
| `GUARDPIPE_ROLE` | `all` | no | `all` \| `api` \| `worker` |
| `GUARDPIPE_HTTP_PORT` | `8080` | no | HTTP listen port |
| `GUARDPIPE_LOG_LEVEL` | `info` | no | `debug` \| `info` \| `warn` \| `error` |
| `GUARDPIPE_BASE_URL` | `http://localhost:8080` | no | Used in links and CORS |

### 5.2 Data

| Variable | Default | Required | Description |
|---|---|---|---|
| `GUARDPIPE_DATABASE_URL` | — | **yes** | `postgres://user:pass@host:5432/guardpipe?sslmode=disable` |
| `GUARDPIPE_DB_MAX_CONNS` | `25` | no | Pool size |
| `GUARDPIPE_REDIS_URL` | — | **yes** | `redis://host:6379/0` |
| `GUARDPIPE_MIGRATE_ON_START` | `true` | no | Apply pending migrations at boot |

### 5.3 Security

| Variable | Default | Required | Description |
|---|---|---|---|
| `GUARDPIPE_JWT_SECRET` | — | **yes** | ≥ 32 bytes. Startup **fails** if shorter |
| `GUARDPIPE_ENCRYPTION_KEY` | — | **yes** | base64 of exactly 32 bytes (AES-256) |
| `GUARDPIPE_ACCESS_TOKEN_TTL` | `15m` | no | |
| `GUARDPIPE_REFRESH_TOKEN_TTL` | `168h` | no | |
| `GUARDPIPE_CORS_ORIGINS` | `http://localhost:5173` | no | Comma-separated allowlist |

### 5.4 Scanning

| Variable | Default | Required | Description |
|---|---|---|---|
| `GUARDPIPE_WORKER_COUNT` | `4` | no | Concurrent jobs |
| `GUARDPIPE_WORKSPACE_ROOT` | `/var/lib/guardpipe/workspace` | no | Ephemeral checkouts |
| `GUARDPIPE_MAX_REPO_MB` | `500` | no | Clone size cap |
| `GUARDPIPE_SANDBOX_MAX` | `2` | no | Concurrent sandbox containers |
| `GUARDPIPE_SANDBOX_IMAGE` | pinned digest | no | Sandbox runner image |
| `GUARDPIPE_DOCKER_HOST` | `unix:///var/run/docker.sock` | no | |
| `GUARDPIPE_ENGINE_TIMEOUT_*` | per [04 §6.3](04-backend-architecture.md#63-timeouts) | no | One per engine |

### 5.5 Pentest

| Variable | Default | Required | Description |
|---|---|---|---|
| `GUARDPIPE_PENTEST_ENABLED` | `true` | no | Master switch |
| `GUARDPIPE_ALLOW_PRIVATE_TARGETS` | `false` | no | **Leave false.** Enables RFC 1918 targets |
| `GUARDPIPE_PENTEST_ALLOWLIST` | empty | no | Optional host allowlist |
| `GUARDPIPE_PENTEST_RATE_LIMIT` | `10` | no | Requests/second ceiling |
| `GUARDPIPE_PENTEST_PORTS` | `top100` | no | Port set |

### 5.6 AI

| Variable | Default | Required | Description |
|---|---|---|---|
| `GUARDPIPE_AI_ENABLED` | `true` | no | Master switch — `false` disables all AI features cleanly |
| `GUARDPIPE_GEMINI_API_KEY` | — | if AI enabled | |
| `GUARDPIPE_GEMINI_MODEL_FAST` | `gemini-2.5-flash` | no | |
| `GUARDPIPE_GEMINI_MODEL_SMART` | `gemini-2.5-pro` | no | |
| `GUARDPIPE_AI_TOKEN_BUDGET_PER_SCAN` | `100000` | no | |
| `GUARDPIPE_AI_CACHE_TTL` | `168h` | no | |

### 5.7 External

| Variable | Default | Description |
|---|---|---|
| `GUARDPIPE_OSV_API_URL` | `https://api.osv.dev` | |
| `GUARDPIPE_OSV_CACHE_TTL` | `24h` | |
| `GUARDPIPE_GITHUB_API_URL` | `https://api.github.com` | |

### 5.8 Gate thresholds

| Variable | Default |
|---|---|
| `GUARDPIPE_GATE_WARN` | `30` |
| `GUARDPIPE_GATE_BLOCK` | `70` |

**Fail-fast validation.** A missing required variable, a short JWT secret, or a wrong-length encryption key aborts startup with a message naming the variable. A security product that boots half-configured is worse than one that refuses to boot.

---

## 6. Secrets handling

| Rule | Detail |
|---|---|
| `.env` is git-ignored, always | Present in `.gitignore` from the first commit |
| `.env.example` is committed | Every variable listed, **all values placeholders** |
| Never a real secret in the repository | Including tests, fixtures, comments, and docs |
| Never a secret in a Docker image layer | Runtime environment only |
| CI secrets in GitHub Actions secrets | Never in workflow YAML |
| Local secrets generated per developer | `make gen-secrets` produces a valid JWT secret and encryption key |
| Rotation | If a secret is ever committed: rotate first, then clean history. Assume compromise regardless |

Generating secrets:
```bash
openssl rand -base64 48
```

---

## 7. Make targets

A single verb per task, so nobody has to remember flags.

| Target | Does |
|---|---|
| `make setup` | Copy `.env.example`, generate secrets, pull images, install deps |
| `make up` / `make down` | Start / stop the stack |
| `make logs` | Follow application logs |
| `make migrate` / `make migrate-down` | Apply / roll back one migration |
| `make migration name=add_x` | Create a new migration file |
| `make seed` | Seed database with fixtures |
| `make test` | All Go tests |
| `make test-unit` | Fast tests only, no containers |
| `make lint` | golangci-lint + eslint |
| `make fmt` | gofmt + prettier |
| `make sqlc` | Regenerate typed queries |
| `make build` | Build both images |
| `make selfscan` | Run GuardPipe against GuardPipe |
| `make demo-prep` | Seed demo data + pre-warm the AI cache |
| `make clean` | Remove containers, volumes, orphan sandboxes |

---

## 8. Continuous integration

### 8.1 Pipeline

```mermaid
flowchart LR
    A[Push / PR] --> B[lint]
    A --> C[test-backend]
    A --> D[test-frontend]
    B & C & D --> E[build images]
    E --> F[container scan]
    F --> G[self-scan]
    G --> H{green?}
    H -->|yes| I[mergeable]
    H -->|no| J[blocked]
```

### 8.2 Jobs

| Job | Runs | Fails the build when |
|---|---|---|
| `lint-backend` | `gofmt -l`, `go vet`, `golangci-lint run` | Any issue |
| `lint-frontend` | `eslint`, `tsc --noEmit`, `prettier --check` | Any issue |
| `test-backend` | `go test ./... -race -coverprofile` with Postgres + Redis services | Any failure; coverage < 60% (NFR-MNT-002) |
| `test-frontend` | `vitest run --coverage` | Any failure |
| `build` | `docker build` for both images | Build failure |
| `container-scan` | Trivy against the built images | HIGH/CRITICAL found |
| `dependency-scan` | `govulncheck` + `npm audit` | HIGH/CRITICAL found (NFR-SEC-009) |
| `self-scan` | GuardPipe scans this repository | Any CRITICAL finding |
| `e2e` (Stretch) | Playwright against the Compose stack | Any failure |

**We use Trivy in CI while building our own container scanner.** That is deliberate and worth saying out loud: Trivy is the independent check on our own supply chain, and having it there also gives us a reference implementation to compare `containerscan`'s output against. Using it in CI is not the same as depending on it in the product ([ADR-0010](17-adr/0010-own-scanners.md)).

### 8.3 Workflow hardening

Our own `cicdscan` rules apply to our own workflows — failing them would be embarrassing and, more usefully, is a real test of the engine:

```yaml
permissions:
  contents: read            # least privilege, explicit (cicdscan.permissions.missing-block)

jobs:
  test:
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683  # v4.2.2
        # pinned to SHA (cicdscan.supply-chain.unpinned-action)
```

- No `pull_request_target`
- No secrets in fork-triggered workflows
- GitHub-hosted runners only
- Concurrency group cancels superseded runs
- Caching for Go modules and npm

### 8.4 Performance

Target: **under 5 minutes** for the full PR pipeline. Jobs run in parallel; Go build and module caches are keyed on `go.sum`; the frontend cache on `package-lock.json`. A slow pipeline gets bypassed by tired people at 2 am — speed is a governance control, not a nicety.

---

## 9. Deployment

### Local / demo

```bash
make setup
make up
make seed
```
Then `http://localhost:5173`.

### Demo-day preparation

Run the day **before**, not the morning of:

- [ ] `make clean && make setup && make up` from a fresh clone — proves the documented path works
- [ ] `make seed` — demo project and fixture repository
- [ ] `make demo-prep` — **pre-warm the AI cache** by running the full demo scan (see [10 §7](10-ai-integration.md#7-caching-fr-ai-008))
- [ ] Verify Gemini quota remaining
- [ ] Run the full demo script end-to-end, timed
- [ ] Screenshot every key screen as a fallback
- [ ] Export a completed scan report as a fallback artifact
- [ ] Confirm the laptop does not depend on venue Wi-Fi for anything but Gemini — and that the cache means it does not need Gemini either

The pre-warm step converts the single largest live-demo risk (external API failure in front of an audience) into a non-event.

---

## 10. Observability

### Logging

Structured JSON to stdout ([04 §9](04-backend-architecture.md#9-logging)). `docker compose logs -f guardpipe` in development; piping to `jq` for filtering.

```bash
docker compose logs -f guardpipe | jq 'select(.level=="error")'
```

### Health

| Endpoint | Meaning |
|---|---|
| `/healthz` | Process alive |
| `/readyz` | Postgres + Redis reachable, migrations applied |
| `/version` | Build version, commit SHA, build time |

### Metrics (Stretch)
Prometheus `/metrics`: HTTP request duration and count, scan duration by engine, queue depth, worker utilisation, AI cache hit rate, external API latency and error rate.

---

## 11. Backup and recovery

| Data | Backup | Recovery |
|---|---|---|
| PostgreSQL | `pg_dump` before any risky migration; `make db-dump` | `make db-restore` |
| Redis | None — losable by design | Rebuilt from PostgreSQL at startup |
| Workspaces | None — ephemeral | N/A |
| Demo data | Committed seed script | `make seed` |

**Before demo day:** take a database dump of the fully-prepared demo state. If anything corrupts during setup, restore in seconds rather than re-running every scan.

---

## 12. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| App exits at startup | Missing required env var | Read the error — it names the variable |
| `/readyz` returns 503 | Postgres or Redis not ready | `docker compose ps`; check health |
| Migrations fail | Two developers claimed the same number | Renumber; see [06 §12](06-database-design.md#12-schema-change-protocol) |
| Scans stay `queued` | Worker not running or Redis unreachable | Check `GUARDPIPE_ROLE`; `redis-cli ping` |
| `containerscan` always skips | Docker socket not mounted | Check the Compose volume mount |
| Sandbox jobs fail immediately | Sandbox image not pulled | `docker pull` the pinned digest |
| AI features return "unavailable" | Missing key or quota exhausted | Check `GUARDPIPE_GEMINI_API_KEY`; check quota |
| Frontend gets CORS errors | Origin not in the allowlist | Set `GUARDPIPE_CORS_ORIGINS` |
| Orphan containers accumulate | Crash before cleanup | `make clean`; startup sweep also handles this |
| Disk fills up | Workspaces not cleaned | `make clean`; check the orphan sweep is running |

---

## 13. Future infrastructure (documented, not built)

Recorded so the architectural intent is visible and so nobody mistakes "not built" for "not considered".

| Phase | Change |
|---|---|
| 1 — now | Docker Compose, local |
| 2 | PostgreSQL → managed (RDS), Redis → managed, images → a registry |
| 3 | Kubernetes: `GUARDPIPE_ROLE=api` Deployment behind an Ingress, `GUARDPIPE_ROLE=worker` Deployment, `/healthz` and `/readyz` as probes |
| 4 | GitOps with ArgoCD; Terraform for infrastructure |

Nothing in the current codebase blocks any of this — which is the entire reason for the role split, the stateless design, the env-only configuration, and the health endpoints being there from day one.
