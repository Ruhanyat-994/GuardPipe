# ADR-0006 — React + Vite SPA over Next.js

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M5, M1 |
| Supersedes | — |

## Context

GuardPipe needs a dashboard: authenticated, data-dense, behind a login, with live scan progress and a large filterable findings table. The backend is a Go REST API.

Constraints:
- One frontend developer, three weeks.
- The backend is Go — "Go is the backend" must remain unambiguous for the deliverable.
- No SEO requirement; every page is behind authentication.
- Live-updating views (2-second polling) and heavy client-side interaction.
- Must deploy in Docker Compose without a second application server.

## Options considered

### Option A — React + Vite SPA
Client-rendered application served as static files, talking to the Go API.

### Option B — Next.js (App Router)
React with server-side rendering, React Server Components, and file-based routing.

### Option C — Server-rendered Go templates (`html/template` + htmx)
No JavaScript build step at all.

### Option D — Vue or Svelte

## Decision

**Option A — React 19 + Vite + TypeScript, built to static files and served by nginx.**

## Rationale

Next.js is an excellent framework solving problems we do not have. Its core value propositions — SEO, fast first paint for public content, server components reducing client bundle size, server-side data fetching — apply to public, content-heavy applications. Every GuardPipe page is behind a login, indexed by nobody, and dominated by client-side interaction with live-updating data.

The decisive objection is architectural rather than technical: Next.js introduces a **second application server** into a stack whose entire premise is "one Go binary". It would need its own container, its own environment configuration, its own deployment concern, and it would blur where the backend actually is. For a project whose architecture is being evaluated, that ambiguity is a real cost.

Option C is genuinely tempting — no build step, no second language, everything in Go. It was rejected on the specific requirements: a virtualised 1,000-row table with multi-dimensional client-side filtering, a live-polling progress view, and an interactive syntax-highlighted diff viewer are all substantially harder without a component framework. htmx is excellent for form-and-list applications; this is closer to an analysis tool.

Option D was rejected purely on team familiarity. Vue and Svelte are fine choices; nobody on this team is faster in them, and three weeks is not the time to find out.

Vite over Create React App or Webpack is not a close decision — sub-second HMR compounds across three weeks of iteration.

## Consequences

### Positive
- Clean separation: Go owns all logic and data, the SPA owns presentation.
- One backend server, unambiguously Go.
- Vite HMR is near-instantaneous.
- Static build output — nginx serves it, no Node runtime in production.
- The full React ecosystem: TanStack Query, TanStack Table, Recharts, shadcn/ui.
- Simple deployment: two containers, no Node process to supervise.

### Negative
- **No SSR** — slower first paint than a server-rendered page. Irrelevant for an authenticated tool, and mitigated by route-level code splitting and a 250 KB budget.
- **No SEO.** Not a requirement.
- **Client-side auth handling is our responsibility** — token refresh, single-flight, redirect-on-401 all had to be designed explicitly ([08 §4.2](../08-frontend-architecture.md#42-token-refresh--single-flight)).
- Two languages and two toolchains in the repository.
- The API contract must be stable early, since the frontend cannot fall back to server-side rendering of the same data.

### Neutral
- Bundle size needs active attention — addressed by budgets and lazy-loading the heavy syntax/diff libraries.

## Revisit when

- A public marketing surface or shareable public report pages are needed → a separate static site or Next.js for that surface only.
- Initial load time becomes a measured problem.
