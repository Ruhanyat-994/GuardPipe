# 09 — UI/UX and Design System

| Field | Value |
|---|---|
| **Document** | UI/UX Design and Design System |
| **Project** | GuardPipe |
| **Version** | 1.6 |
| **Status** | Draft |
| **Tool** | Figma |
| **Owner** | Member 6 (design) with Member 5 (frontend) |
| **Last updated** | 2026-07-31 |

### Revision history

| Version | Date | Author | Change |
|---|---|---|---|
| 1.0 | 2026-07-29 | Team | Initial design system and screen specification |
| 1.1 | 2026-07-31 | Team | Added the public site (Landing, Blog, Guides) as a second, distinct surface alongside the authenticated app — direction adapted from tridentsecurity.io — plus the severity stat-tile treatment for the dashboard (§5.1), adapted from Tenable/SecurityCenter-style scanner dashboards |
| 1.2 | 2026-07-31 | Team | **Specification only — not yet built** (see `BUILD_GUIDE.md` Phase 9). Adopted a Jira-inspired direction for the authenticated app's *chrome* (§4.4): dark top bar, collapsible icon+label sidebar, per-project tab bar, right-anchored overlay panels for global search/notifications/user menu, neutral stat cards alongside the existing severity tiles. Added §2.10, the light/dark/system theme-mode requirement, surfaced through the new `UserMenu` → `Theme` control exactly as Jira does it. The Phase 3 sidebar/dashboard chrome already shipped is a functional placeholder against this spec, not the final design — routes and nav items stay, styling and a few new chrome pieces (tab bar, search overlay, notifications, context menus, theme switcher) do not exist in code yet |
| 1.3 | 2026-07-31 | Team | **Implemented in code**, pulled forward from Phase 9 (the design work in v1.2 landed the same day, at the user's explicit request, ahead of Phases 4–8's backend work — see `PROGRESS-LOG.md`). All 9 §4.4 components built (`TopBar`, `SidebarNav`, `ProjectTabBar`, `UserMenu`, `ThemeSubmenu`, `NotificationPanel`, `GlobalSearch`, `ContextMenu`, `MetricCard`), §2.10's light/dark/system theme switcher working end to end with no flash-of-wrong-theme, and the `ContextMenu`/`ProjectSettingsPage` "Archive project" action wired to the `project.Service.Archive` endpoint that had been backend-ready with no UI since Phase 3. `GlobalSearch` and `NotificationPanel` ship intentionally scoped down from the full spec — see the deviations noted inline in §4.4 |
| 1.4 | 2026-07-31 | Team | **Implemented in code.** User feedback: the projects list/creation screens read as empty and generic, missing the "actual small logo" polish GitHub's own UI has. Added §4.5 (provider brand marks — `GitHubMark`, a real inline GitHub logomark used everywhere a GitHub repository is shown, replacing the generic folder icon; explicitly **not** adding a non-functional GitLab/Bitbucket option since only GitHub is implemented) and §4.6 (progressive-disclosure step badges + a `.animate-reveal` transition for `ProjectCreatePage`'s later sections). Enriched the Screen 3 project cards (§5.6) with a real per-card `ContextMenu`, default branch, and relative creation time, and the repository-attach result card with a real outbound link to the repository plus branch/visibility badges |
| 1.5 | 2026-07-31 | Team | **Implemented in code.** User feedback with a reference screenshot: Login/Register "looked weird," redesigned as a split-screen (§5.8) — direction adapted from Aikido's login, fixed-white form panel + fixed-dark info panel (`AuthShell`, `AuthSidePanel`), same layout as the reference but a real email/password form instead of OAuth buttons (not built) and real GuardPipe claims instead of fabricated trust badges/customer logos (none exist). New tokens: `--color-auth-panel-bg`/`-fg`/`-fg-secondary` plus a `.auth-panel-light` class that locally re-pins every theme-relative token to its light value within the panel, so `Input`/`Button` render consistently regardless of the visitor's theme preference — the same mechanism `.dark` uses, scoped down instead of applied to `<html>` |
| 1.6 | 2026-07-31 | Team | **Implemented in code.** User supplied GuardPipe's actual logo artwork (a shield containing a run/checkpoint/stop pipeline graphic) with instructions to set it as the brand mark everywhere, background removed. Added §4.7. Background removal done programmatically (border flood-fill through connected near-white pixels, not a blanket colour-key, so the logo's own white line-art survived) since no image-editing tool was available in this session. Replaced the Vite-scaffold placeholder favicon (still in place since Phase 0) with real favicons generated from the new mark. Wired the new `Logo` component into `AppShell`, `AuthShell`, and `PublicNav` |

> This document is the **specification for the Figma deliverable** and the design contract for the frontend. The Figma file is built from this; the frontend is built from both.

---

## 1. Design principles

| # | Principle | What it means in practice |
|---|---|---|
| 1 | **Severity is the primary axis** | Every list, chart, and card sorts and groups by severity first. A user should never hunt for the critical finding |
| 2 | **Never colour alone** | Severity = colour **+** icon **+** text label, everywhere, without exception (FR-UI-008) |
| 3 | **Plain language before jargon** | "An attacker could read your database" comes before "CWE-89 SQL Injection" |
| 4 | **Show the fix, not just the flaw** | Every finding detail leads with remediation, not with blame |
| 5 | **Honest about uncertainty** | AI content is labelled. Partial scans say so. Low-confidence findings say so |
| 6 | **Density with breathing room** | This is an analysis tool — users scan hundreds of rows. Compact, but never cramped |
| 7 | **Calm, not alarming** | A dashboard that screams red at everything gets ignored. Reserve the loudest treatment for genuine criticals |
| 8 | **Familiar chrome, novel content** | The app's *navigation shell* — top bar, sidebar, tabs, menus, search, notifications — should feel like software people already know how to drive. Save the novelty budget for the parts that are actually GuardPipe's (severity treatment, the pipeline, the AI patch panel), not for reinventing a settings dropdown |

Principle 7 is the hardest to hold. Security UI defaults to a wall of red; the discipline is to make `critical` visually rare so that when it appears, it lands.

**Principle 8, concretely:** the authenticated app's chrome follows Jira's own product patterns — dark top bar, light collapsible sidebar with icon+label groups, a per-project tab bar, right-anchored overlay panels for search/notifications/the user menu, neutral (non-severity-coloured) stat cards for metrics that aren't severity counts. Full specification in §4.4 and the updated §5.1 wireframe. This is a **chrome/interaction-pattern** borrowing, not a content one: GuardPipe keeps its own navigation vocabulary (Projects/Scans/Findings/Targets/Rules), its own accent colour, and the severity system from principles 1–2 untouched — only *how the shell around that content behaves* changes. **Built** — see rev 1.3 and §4.4.

**One deliberate exception:** the five severity stat tiles at the top of the project dashboard (§5.1) are solid, fully-saturated severity-colour blocks — the Tenable/SecurityCenter-style "scannable from across the room" treatment. This is the single loud moment in the product, chosen because it is the literal answer to "can this ship?" Everything below that fold (badges, borders, table rows) returns to the restrained colour-as-accent treatment principle 7 describes.

**Two surfaces, two voices.** Everything above applies to the **authenticated product** (the dashboard and everything under it). The **public site** — Landing, Blog, Guides, described in §5.9 — is a separate marketing/education surface with its own typographic voice (serif display type, a darker hero, motion) and does not use the severity system at all. A visitor reading a guide article should not see it dressed as a security tool's cockpit; a user triaging findings should never see marketing chrome. Keep the two visually distinct on purpose.

---

## 2. Design tokens

### 2.1 Colour — base palette

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg-base` | `#FAFAFA` | `#0B0D10` | App background |
| `--bg-surface` | `#FFFFFF` | `#14171C` | Cards, panels |
| `--bg-surface-raised` | `#FFFFFF` | `#1B1F26` | Modals, popovers |
| `--bg-subtle` | `#F4F5F7` | `#1B1F26` | Table header, hover |
| `--border-default` | `#E3E5E9` | `#272C34` | Dividers, card borders |
| `--border-strong` | `#C8CCD4` | `#3A414C` | Input borders |
| `--text-primary` | `#12141A` | `#F2F4F7` | Headings, body |
| `--text-secondary` | `#5A6270` | `#A2ABBA` | Labels, metadata |
| `--text-tertiary` | `#8A93A2` | `#6E7889` | Placeholders, timestamps |
| `--text-inverse` | `#FFFFFF` | `#0B0D10` | On filled buttons |

### 2.2 Colour — severity (the most important tokens in the system)

| Severity | Token | Light | Dark | Icon | Contrast on surface |
|---|---|---|---|---|---|
| Critical | `--sev-critical` | `#B4232A` | `#FF6B72` | `octagon-alert` | 6.9:1 / 7.2:1 |
| High | `--sev-high` | `#C2410C` | `#FF8B4C` | `triangle-alert` | 5.1:1 / 6.8:1 |
| Medium | `--sev-medium` | `#A16207` | `#F5B93B` | `alert-circle` | 4.9:1 / 8.4:1 |
| Low | `--sev-low` | `#1D4ED8` | `#7BA7FF` | `info` | 6.4:1 / 6.6:1 |
| Informational | `--sev-info` | `#4B5563` | `#9AA5B4` | `circle-dot` | 7.6:1 / 6.1:1 |

Each has a matching `--sev-*-bg` (10% tint) for badge backgrounds and a `--sev-*-border`.

> **Deliberate choice:** high is **orange**, not a second red. The single most common failure in security dashboards is critical and high being indistinguishable at a glance. Distinct hue + distinct icon + text label makes them separable for every user, including monochrome vision.

### 2.3 Colour — semantic

| Token | Light | Dark | Use |
|---|---|---|---|
| `--accent` | `#2563EB` | `#5B8DEF` | Primary actions, links, focus ring |
| `--success` | `#15803D` | `#4ADE80` | Pass verdict, fixed findings |
| `--warning` | `#A16207` | `#F5B93B` | Warn verdict, degraded state |
| `--danger` | `#B4232A` | `#FF6B72` | Block verdict, destructive actions |
| `--ai` | `#7C3AED` | `#A78BFA` | AI-generated content marker |

`--ai` purple is used *only* for AI-attributed content — the explanation panel border, the patch banner, the "AI" chip. One colour, one meaning, so users learn instantly which content came from a model (FR-AI-012).

### 2.4 Verdict colours

| Verdict | Token | Meaning |
|---|---|---|
| `pass` | `--success` | Safe to ship |
| `warn` | `--warning` | Ship with awareness |
| `block` | `--danger` | Do not ship |

### 2.5 Typography

| Token | Family | Size / line-height | Weight | Use |
|---|---|---|---|---|
| `--font-sans` | Inter | — | — | All UI |
| `--font-mono` | JetBrains Mono | — | — | Code, paths, hashes, diffs |
| `text-display` | sans | 32 / 40 | 600 | Page title |
| `text-h1` | sans | 24 / 32 | 600 | Section |
| `text-h2` | sans | 20 / 28 | 600 | Card title |
| `text-h3` | sans | 16 / 24 | 600 | Subsection |
| `text-body` | sans | 14 / 22 | 400 | Default |
| `text-body-sm` | sans | 13 / 20 | 400 | Table cells |
| `text-caption` | sans | 12 / 16 | 400 | Metadata, timestamps |
| `text-code` | mono | 13 / 20 | 400 | Snippets, file paths |

**14 px body, not 16.** This is a dense analysis tool; 16 px pushes too little information above the fold in a findings table. 12 px is the floor.

### 2.6 Spacing — 4 px base scale

`space-1` 4 · `space-2` 8 · `space-3` 12 · `space-4` 16 · `space-5` 20 · `space-6` 24 · `space-8` 32 · `space-10` 40 · `space-12` 48 · `space-16` 64

| Context | Value |
|---|---|
| Icon ↔ text | `space-2` |
| Inside a card | `space-6` |
| Between cards | `space-4` |
| Between sections | `space-8` |
| Table cell padding | `space-3` vertical, `space-4` horizontal |
| Page gutter | `space-8` |

### 2.7 Radius, elevation, motion

| Token | Value | Use |
|---|---|---|
| `radius-sm` / `md` / `lg` / `full` | 4 / 8 / 12 / 9999 px | badges / cards, inputs / modals / pills |
| `shadow-sm` | `0 1px 2px rgb(0 0 0 / .05)` | Cards |
| `shadow-md` | `0 4px 12px rgb(0 0 0 / .08)` | Dropdowns, popovers |
| `shadow-lg` | `0 12px 32px rgb(0 0 0 / .12)` | Modals |
| `duration-fast` / `base` / `slow` | 120 / 200 / 320 ms | Hover / panels / page |
| `ease-standard` | `cubic-bezier(.2,0,0,1)` | Everything |

All motion is disabled under `prefers-reduced-motion: reduce`.

### 2.8 Layout

| Element | Value |
|---|---|
| Sidebar | 240 px expanded, 64 px collapsed |
| Top bar | 48 px, **always the dark/inverse chrome colour**, independent of the app's own light/dark theme mode (§2.10) — Jira-style constant landmark |
| Project tab bar | 44 px, sits directly below the top bar once inside a project (§4.4 `ProjectTabBar`) |
| Content max width | 1440 px, centred |
| Detail drawer | 480 px |
| Grid | 12 columns, 24 px gutter |

### 2.9 Public-site tokens (Landing, Blog, Guides only — never used inside the authenticated app)

The public site borrows the base palette and spacing scale above but adds its own display type and background treatment. These tokens live in a separate `public.css` layer, not `globals.css`, so they can never leak into product screens by accident.

| Token | Value | Use |
|---|---|---|
| `--font-display-serif` | Fraunces (fallback: Georgia, serif) | Hero and section headlines on Landing/Blog/Guides only |
| `--public-bg-hero` | `#05070C` | Landing hero background — deliberately darker than `--bg-base` dark (`#0B0D10`) so the marketing surface can read as its own brand moment |
| `--glow-1` / `--glow-2` | `#3B5BFF` / `#1E1B4B` | Radial gradient stops for the animated hero glow |
| `--public-accent-critical` | `#B4232A` | The single small red accent dot used sparingly in hero copy (e.g. a live-status pulse) — same hex as `--sev-critical` but **not** wired to any severity meaning here, it is purely decorative |
| `text-display-hero` | serif, 56 / 64, 500 | Landing hero headline |
| `text-display-section` | serif, 40 / 48, 500 | Blog/Guides section headlines |
| `--motion-glow-duration` | 18s, `linear`, infinite loop | Hero background animation |

**Motion rule:** under `prefers-reduced-motion: reduce`, the glow animation is replaced with its static midpoint frame — never fully removed, since the gradient is also the background's visual weight, but never animated for a user who asked not to see it. This is the same rule as §2.7, applied to a second surface.

**Why a separate serif at all:** principle 3 ("plain language before jargon") and the density goal in principle 6 are product-UI concerns — a findings table needs to show hundreds of rows, a landing page needs to sell an idea in one screen. Borrowing Trident's editorial serif-over-sans pairing for the public site only, while keeping Inter everywhere the product itself renders data, keeps both jobs honest instead of compromising one for the other.

### 2.10 Theme modes — light / dark / system

**Built (rev 1.3).** Every colour token in §2.1–§2.4 already ships a light and a dark value; the switcher and the system-follows behaviour now exist too (`stores/themeStore.ts`, an inline boot script in `index.html`, `UserMenu`'s `ThemeSubmenu`). Three modes, not two — a binary light/dark toggle silently drops the option most users actually want:

| Mode | Behaviour |
|---|---|
| **Light** | Forces the light column of every token |
| **Dark** | Forces the dark column |
| **System** (default) | Follows the OS/browser `prefers-color-scheme` media query **live** — changing the OS theme while GuardPipe is open updates the app immediately, no reload |

- **Reachable from `UserMenu` → `Theme`** (§4.4), a three-option radio-style submenu — exactly Jira's own pattern (avatar menu → `Theme` → `Light` / `Dark` / `System`), not a single sun/moon toggle button in the top bar.
- An explicit user choice (Light or Dark) is **persisted** (e.g. `localStorage` now; a real `identity`/account-settings field once one exists) and overrides `prefers-color-scheme` until the user picks System again or clears it.
- **No flash of the wrong theme on load.** The mode must be resolved and the theme class/attribute applied *before first paint* — an inline script in `index.html`, not a post-mount `useEffect` — the same discipline already required for `prefers-reduced-motion` (§2.7).
- "System" is not a fourth visual design — it always resolves to the existing light or dark token set at runtime. Nothing new to design per component; the new work is the switcher UI, the persistence, and the no-flash boot logic.
- Applies to the **entire product**: the authenticated app and the public site (§2.9) alike. The public site's hero (§2.9) is deliberately dark regardless of mode (it's a fixed brand moment, not a themed surface); its lighter sections below the hero do respond to the mode switch.
- §8's "dark mode contrast verified independently" checklist item extends to: verify there is no flash-of-wrong-theme on a hard reload, in all three modes, in at least one Chromium and one non-Chromium browser.

---

## 3. Figma file structure

**File name:** `GuardPipe — Design System & Screens v1`

| Page | Contents | Owner |
|---|---|---|
| `00 · Cover` | Project name, team, version, changelog, link to this document | M6 |
| `01 · Foundations` | Colour styles, type styles, spacing scale, radius, elevation, icon set — all as **published Figma styles/variables**, not loose rectangles | M6 |
| `02 · Components` | The component library (§4), each with all variants and states | M6 |
| `03 · Patterns` | Loading / empty / error / partial state patterns; severity treatment; AI content treatment | M6 |
| `04 · Wireframes` | Low-fidelity layouts of all 12 screens — **build these first** | M6 |
| `05 · Screens — Desktop` | High-fidelity, 1440 × 1024 frames | M6 |
| `06 · Screens — Tablet` | 768 px variants of the 4 primary screens only | M6 |
| `07 · Prototype` | Clickable flow for the demo path | M6 |
| `08 · Handoff` | Redlines, token mapping table, component→code name map | M6 + M5 |
| `09 · Public Site` | Landing, Blog index/post, Guides index/detail — high-fidelity, own moodboard section (§5.9) | M6 |

### Conventions
- **Auto Layout on every frame.** A design that cannot resize cannot be built.
- **Component variants, not duplicated frames.** `Badge/Severity/Critical`, not `badge-critical-copy-3`.
- **Figma Variables for all tokens**, with light and dark modes — token names match the CSS custom property names in §2 exactly. This is what makes handoff mechanical instead of interpretive.
- **Naming:** `Category/Component/Variant` — `Button/Primary/Default`, `Table/Row/Hover`.
- Every screen frame is named for its route: `Screen — Findings Explorer (/scans/:id/findings)`.

---

## 4. Component inventory

### 4.1 Primitives (map to shadcn/ui)

`Button` (primary · secondary · ghost · destructive × default/hover/active/disabled/loading, sm/md/lg) · `Input` · `Textarea` · `Select` · `MultiSelect` · `Checkbox` · `Radio` · `Switch` · `Badge` · `Chip` · `Avatar` · `Tooltip` · `Dialog` · `Drawer` · `DropdownMenu` · `Tabs` · `Accordion` · `Progress` · `Skeleton` · `Toast` · `Breadcrumb` · `Pagination` · `Command` (⌘K search)

### 4.2 Domain components

| Component | Variants / states | Notes |
|---|---|---|
| `SeverityBadge` | 5 severities × (sm, md) × (with count, without) | Colour + icon + label. The most-used component in the system |
| `SeverityStatTile` | 5 severities, each a solid colour block, big number + label, small trend-delta chip in the top-right corner | The one "loud colour" exception (principle 7) — modelled on Tenable/SecurityCenter-style headline counts. Used only in the dashboard's top row (§5.1) |
| `StatusPill` | open · acknowledged · suppressed · fixed · false_positive | Neutral colours — status is not severity |
| `EngineIcon` | 7 engines | Consistent glyph per engine, used in tables, filters, and the pipeline |
| `RiskGauge` | 0–100 arc, verdict band, delta arrow, 3 sizes | Hero element of the dashboard |
| `SupplyChainPipeline` | 7 stages × (succeeded / failed / skipped / running / not_run) | The signature visual (FR-UI-004) |
| `StageCard` | Per-engine card: status, worst severity, count, duration | Clickable → filters findings |
| `FindingRow` | default · hover · selected · suppressed (dimmed) | Virtualised list row |
| `CodeBlock` | With line numbers, highlighted range, copy button | Shiki-rendered |
| `PatchDiff` | unified · split; verified · unverified badge | Always carries the AI banner |
| `AiPanel` | loading · content · unavailable · budget-exhausted | Purple left border, "AI-generated" chip |
| `CvssChip` | score + severity colour, vector on hover | |
| `CweChip` / `CveChip` | Links to MITRE / NVD | |
| `TrendChart` | Line, score over time, verdict bands | Recharts |
| `SeverityDonut` | 5-segment distribution with a centre total | Recharts |
| `EmptyState` | icon + title + description + action | 6 written variants |
| `ErrorState` | message + retry + `request_id` | |
| `PartialResultBanner` | names the failed/skipped engines | The state people forget |

### 4.3 Public-site components (Landing, Blog, Guides — §5.9)

These use §2.9's tokens and never appear inside the authenticated app.

| Component | Variants / states | Notes |
|---|---|---|
| `PublicNav` | default · scrolled (condensed) | Floating pill nav, sticky; logo + Guides + Blog + primary CTA |
| `HeroGlow` | animated · static (reduced-motion) | Absolutely-positioned animated gradient behind hero copy |
| `MarketingCard` | with-media · text-only | The dual-column feature cards on the Landing page |
| `BlogCard` / `GuideCard` | default · featured (larger, first item) | Eyebrow category label + serif title + one-line description + meta |
| `GuideSidebar` | default · active-section highlighted | Sticky in-page section nav for a guide's steps |
| `MarketingCtaBand` | default | Full-width closing call-to-action band with one button |

### 4.4 Authenticated app shell components — Jira-inspired (built)

**Status: built in code (rev 1.3), pulled forward from Phase 9 — see `PROGRESS-LOG.md`.** Direction: Atlassian/Jira's own product chrome — dark top bar, light collapsible sidebar, tab-based per-project sub-navigation, dense neutral stat/card grids, right-anchored overlay panels for search/notifications/menus. **Adapted, not copied** (principle 8, §1): GuardPipe keeps its own navigation vocabulary (Projects/Scans/Findings/Targets/Rules — never Jira's Spaces/Boards/Backlog), its own single accent blue (§2.3), and drops anything tied to Jira's own product model GuardPipe has no equivalent of (multi-space switching, a premium-trial upsell pill, a standalone "Ask Rovo"-style AI-chat entry point — GuardPipe's AI surfaces live inside finding detail, §5.3, not as a global chat affordance). These components **replace the visual styling** of the Phase 3 `AppShell` sidebar; the navigation items, routes, and `RequireAuth` wrapping already built in Phase 3 did not change.

| Component | Variants / states | Notes |
|---|---|---|
| `TopBar` | default · search-focused (expanded) | Fixed, 48 px (§2.8), always the dark/inverse chrome colour regardless of the app's own theme mode (§2.10) — a constant landmark, exactly like Jira's black bar surviving both its light and dark modes. Houses, left to right: sidebar-collapse toggle, wordmark, `GlobalSearch`, a context-dependent primary action ("+ New Project" / "+ New Scan"), notification bell (`NotificationPanel` trigger), help icon (links to `/guides`), `UserMenu` trigger. **Built** — `components/AppShell.tsx` |
| `SidebarNav` | expanded (240 px) · collapsed (64 px, icon-only, native `title` tooltip) | Icon + label rows in two groups separated by a divider: primary (Projects · Scans · Findings · Targets · Rules) then secondary (Settings) — grouping unchanged from Phase 3, styling upgraded to match Jira's icon weight/spacing/hover treatment. **Built** |
| `ProjectTabBar` | default | Sits directly below `TopBar` (§2.8, 44 px), above page content, once inside a project: breadcrumb ("Projects") + project name + `ContextMenu` trigger, then an underlined tab row (Overview · Scans · Findings · Targets · Settings). Active tab = `--accent` text + 2 px underline + filled icon. **Built** — `components/project/ProjectTabBar.tsx`, wraps a new `ProjectLayout` nested-route layout (`pages/ProjectLayout.tsx`) |
| `UserMenu` | closed · open | Avatar trigger (colour-coded circle, initials) opens a panel: identity header (avatar, display name, email) → divider → `Profile` · `Account settings` · **`ThemeSubmenu`** (§2.10) → divider → `Log out`. **Built** — `Profile`/`Account settings` both currently route to the `/settings` placeholder (no dedicated profile screen exists yet) |
| `ThemeSubmenu` | Light · Dark · System (single-select, one always checked) | The concrete UI for §2.10 — an inline-expanding nested list off `UserMenu` → `Theme` (not a separate flyout, to avoid nested-overlay dismiss complexity), not a top-bar toggle button. **Built** |
| `NotificationPanel` | empty | Slide-in panel anchored under the bell icon. **Built, intentionally scoped down**: ships as a permanent, honest empty state ("You're all caught up") rather than the full populated/grouped-by-date/unread-toggle spec above — there is no notification-producing backend yet (that needs the orchestrator, Phase 6, plus a notifications table that doesn't exist). The populated states remain the target once that data source exists |
| `GlobalSearch` | collapsed (pill, in `TopBar`) · expanded (overlay) | **Built, intentionally scoped down**: a real, working client-side filter over the caller's own projects (fetched once, filtered by name as you type), not the full interpreted-query/filter-chip/quick-category spec above — there is no cross-entity (findings/rules) search backend yet. Chosen over a fabricated "AI-interpreted query" row, which would have been dishonest sample content |
| `ContextMenu` | per-object: project (scan · finding once those exist) | Right-aligned `···` trigger → grouped list: primary actions first, then a divider, then destructive actions in `--danger` at the bottom. **Built** — `Archive project` is the first real use, calling `project.Service.Archive`, which had been backend-ready with no frontend control since Phase 3, and now appears both inside a project (`ProjectTabBar`) and per-card on the projects list (§5.6). Confirmation is `window.confirm` for now, not a styled `Dialog` (§4.1's `Dialog` primitive isn't built yet) |
| `MetricCard` | default | Neutral `--bg-surface` stat card: number + label + optional info-icon tooltip, **no** colour coding. Distinct from `SeverityStatTile` (§4.2), which stays the one deliberate loud-colour exception for severity counts specifically (principle 7). **Built** — component exists (`components/ui/MetricCard.tsx`); not yet placed on a real screen, since every current count (projects, targets) reads fine as plain text at today's scale — first real usage will likely be the dashboard once Phase 6/8 give it non-severity metrics worth a card |

### 4.5 Provider brand marks (built)

**Rule:** anywhere the UI shows "this repository/resource lives on X," it shows X's real logomark — never a generic folder/link icon standing in for a specific brand. A generic icon says "there is a repository here"; a brand mark says "there is a *GitHub* repository here," which is the more useful and more trustworthy signal (this was explicit user feedback: "GitHub itself [is] a very designed [product] ... you need to follow that").

- **`GitHubMark`** (`components/icons/GitHubMark.tsx`) — GitHub's own Octicons `mark-github` glyph, inline SVG, `fill="currentColor"` so it never needs its own light/dark colour handling. Used: the repository field on every project card (§5.6), the repository-URL input's leading icon, and the attached-repository result card (`RepositoryAttachForm`, used by both `ProjectCreatePage` and `ProjectSettingsPage`) — the attached card links out to the real repository URL (`target="_blank"`), not a dead link.
- **Only GitHub exists today.** `repositories.provider` is schema-extensible (`documentation/06-database-design.md` §4.5: `CHECK (provider IN ('github'))`, "extensible without a type migration") but `adapters/github` is the only implemented provider. **Do not add a GitLab/Bitbucket option to any picker until a second provider is actually implemented on the backend** — a provider selector that only functions for one of its options is exactly the kind of fake-functionality the rest of this document argues against (see `GlobalSearch`/`NotificationPanel` in §4.4). When a second provider lands, its own brand mark gets the same treatment as `GitHubMark` — same component shape, new SVG.

### 4.6 Progressive disclosure and reveal motion (built)

Pattern for any form where later fields only make sense once an earlier step is committed (the canonical case: `ProjectCreatePage` — Repository and Pentest Target only make sense once a Project exists to attach them to).

- **Numbered step badges** — a small filled-accent circle with the step number, becoming a `--success`-coloured checkmark once that specific step is actually done (not just "revealed"; skippable steps like an optional repository attach never show "done," only their number, since there's no false completion signal to give). Kept lightweight — this is not a `Stepper` primitive with lines connecting circles, just a per-section label, since GuardPipe's forms aren't literally sequential wizards (every later step stays independently skippable).
- **Reveal transition** — newly-shown sections fade and slide up slightly (`.animate-reveal`, `index.css`, `--duration-slow`/`--ease-standard` from §2.7) rather than snapping into existence. Automatically covered by the existing global `prefers-reduced-motion` rule — no separate reduced-motion variant needed, same as `HeroGlow`'s drift animation.
- This is the "special effects... buttons that mean something with the project" feedback in practice: motion and step state exist to communicate *progress through a real process*, not decoration. A button whose loading/success/disabled state doesn't map to anything real is exactly what this pattern avoids.

### 4.7 Brand mark (built)

`components/Logo.tsx` — the shield ("guard") containing a connected run → checkpoint → decision → stop pipeline graphic ("pipe"), user-supplied artwork. Source was a raster PNG on a near-white background; background removed programmatically (flood-fill from the canvas border through connected near-white pixels only — not a blanket colour-key, which would have also erased the white pipeline line-art *inside* the shield — plus a short feather pass on the boundary ring to avoid a hard/jagged edge), then trimmed to its bounding box and re-encoded at a UI-appropriate resolution. Transparent PNG (`src/assets/logo.png`), `object-contain`-safe sizing via height only.

**Paired with the "GuardPipe" wordmark at every call site** (`AppShell` top bar, `AuthShell`, `PublicNav`) — this is the icon half of a lockup, not a standalone logo-with-text asset. Also supplies the favicon (`public/logo-{32,180,512}.png`, replacing the default Vite-scaffold placeholder that was still in place) — `index.html` references all three sizes (standard favicon, Apple touch icon, and a large PNG for anywhere a high-res app icon gets pulled).

**Recoloured to match `--accent`** (was the source artwork's original green) — a vectorised hue rotation in HSV space (shift every pixel's hue by the delta between the artwork's dominant green and `--accent`'s hue, leaving saturation/value untouched) rather than a flat colour swap, so the shield's existing gradient/shading survives instead of flattening into one solid blue. The dark inner panel and white line-art are low-saturation, so the rotation leaves them essentially unaffected — only the green regions (the shield body and the small icon-fill accents) actually move.

---

## 5. Screen specifications

### Screen inventory

| # | Screen | Route | Priority |
|---|---|---|---|
| 1 | Login | `/login` | P0 |
| 2 | Register | `/register` | P1 |
| 3 | Projects list | `/projects` | P0 |
| 4 | Project dashboard | `/projects/:id` | **P0 — hero screen** |
| 5 | New Scan wizard | `/projects/:id/scans/new` | P0 |
| 6 | Scan progress | `/scans/:id` (running) | P0 |
| 7 | Scan results | `/scans/:id` (completed) | P0 |
| 8 | Findings explorer | `/scans/:id/findings` | **P0 — hero screen** |
| 9 | Finding detail | `/scans/:id/findings/:fid` | **P0 — hero screen** |
| 10 | Pentest targets | `/projects/:id/targets` | P1 |
| 11 | Rules catalogue | `/rules` | P2 |
| 12 | Settings | `/settings` | P2 |
| 13 | Landing | `/` | **P1 — public, first thing anyone sees** |
| 14 | Blog index | `/blog` | P1 |
| 15 | Blog post | `/blog/:slug` | P1 |
| 16 | Guides index | `/guides` | P1 |
| 17 | Guide detail | `/guides/:slug` | P1 |

**Design order: 4 → 8 → 9 → 6 → 5 → 3 → the rest.** The three hero screens carry the entire demo; if only three screens reach high fidelity, these are the three. Screens 13–17 (the public site, §5.9) are ranked P1 rather than P0 because they don't gate the scanning flow — but do them early anyway if a demo/checkpoint deadline is close, since a landing page is the single highest-leverage thing to show someone in the first ten seconds of a demo.

---

### 5.1 Screen 4 — Project dashboard (hero)

**Purpose:** answer "can this ship?" in under five seconds.

**Chrome status (rev 1.3): built.** The wireframe below is what's actually in code now — the Jira-inspired `TopBar`/`ProjectTabBar` chrome from §4.4, replacing Phase 3's plain light placeholder header. The content below the tab row (risk gauge, severity tiles, pipeline, findings, trend) is unchanged by this revision — it is still Phase 2/3's hardcoded sample-data preview, only the chrome around it is new.

```
┌──────────────────────────────────────────────────────────────────────────┐
│▓▓ ☰  GuardPipe      🔍 Search…              [+ New Scan]  🔔  ?  👤 ▓▓▓▓▓│  ← TopBar (dark, always)
├────────┬─────────────────────────────────────────────────────────────────┤
│        │ Projects › Payments API                                 ···  ⤢ │  ← ProjectTabBar: breadcrumb + ContextMenu
│ Proj.  │ Overview   Scans   Findings   Targets   Settings                │  ← underlined tabs, Overview active
│ Scans  │─────────────────────────────────────────────────────────────────│
│ Find.  │  Payments API                          Last scan 4 min ago      │
│ Targ.  │  github.com/acme/payments-api · main · a3f9c21                  │
│ Rules  │                                                                 │
│ ─────  │  ┌──────────────┐  ┌─────────────────────────────────────────┐ │
│ Sett.  │  │              │  │ ▲2 Critical  ▲9 High  ●21 Med  ●14 Low  │ │
│        │  │   ◕  68      │  │                                          │ │
│        │  │   BLOCK      │  │      [ severity donut chart ]            │ │
│        │  │   ▼ 6 ↓      │  │                                          │ │
│        │  └──────────────┘  └─────────────────────────────────────────┘ │
│        │                                                                 │
│        │  Supply Chain                                                   │
│        │  ┌────┐  ┌────┐  ┌────┐  ┌────┐  ┌────┐  ┌────┐  ┌────┐        │
│        │  │Docs│──│Code│──│Deps│──│Cont│──│ K8s│──│CICD│──│Pent│        │
│        │  │ ⚠  │  │ ▲18│  │ ▲11│  │ ⊘  │  │ ▲14│  │ ▲6 │  │ –  │        │
│        │  │fail│  │high│  │crit│  │skip│  │crit│  │high│  │n/r │        │
│        │  └────┘  └────┘  └────┘  └────┘  └────┘  └────┘  └────┘        │
│        │                                                                 │
│        │  ⚠ Document review failed: AI service unavailable.  [Retry]     │
│        │                                                                 │
│        │  ┌─ Top findings ─────────────┐ ┌─ Risk trend ────────────────┐│
│        │  │ ▲ Hardcoded AWS key     🔴 │ │      ╲                       ││
│        │  │ ▲ cluster-admin binding 🔴 │ │  74 ──╲── 68                 ││
│        │  │ ▲ SQL injection         🟠 │ │        ╲                     ││
│        │  │                [View all →]│ │  [last 20 scans]             ││
│        │  └────────────────────────────┘ └──────────────────────────────┘│
└────────┴─────────────────────────────────────────────────────────────────┘
```

| Element | Detail |
|---|---|
| `TopBar` | §4.4 — dark chrome, search, `+ New Scan`, notifications, `UserMenu`. **Built** |
| `ProjectTabBar` | §4.4 — breadcrumb, `ContextMenu` (`···`, includes `Archive project`, backend-ready since Phase 3), tab row. **Built** |
| Risk gauge | 0–100 arc, verdict word, delta vs previous scan with direction arrow |
| Severity stat tiles | Five `SeverityStatTile` blocks (critical/high/medium/low/info) above the donut — big number, label, small delta chip (e.g. "-18") in the corner showing the change since the previous scan. Modelled directly on the Tenable/SecurityCenter reference: scannable at a glance, no need to read the donut to know if there's a critical |
| Severity summary | Donut alongside the tiles for proportion at a glance; every tile and every donut segment is clickable → filtered findings |
| Supply chain pipeline | 7 stages; colour = worst severity; icon = job status; click → findings filtered by engine |
| Partial banner | Present whenever any job is `failed` — never hidden |
| Top findings | 5 highest-severity open findings |
| Trend | Last 20 scans, with verdict bands as background |
| States | loading (skeleton) · empty ("No scans yet" + CTA) · error · partial |

### 5.2 Screen 8 — Findings explorer (hero)

**Purpose:** find the finding that matters among hundreds.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ← Scan 7d3f… · Payments API · completed 10:23           [Export ▾]       │
├──────────────────────────────────────────────────────────────────────────┤
│ 🔍 search…    Severity ▾  Engine ▾  Status ▾  CWE ▾     53 findings      │
│ ⌫ critical ×  high ×  codescan ×  open ×               [Clear all]       │
├──────────────────────────────────────────────────────────────────────────┤
│ Sev ▾ │ Finding                        │ Engine │ Location      │ Age    │
├───────┼────────────────────────────────┼────────┼───────────────┼────────┤
│ 🔴 CRT│ Hardcoded AWS access key       │ code   │ config/aws.go │ 4d  ›  │
│ 🔴 CRT│ ClusterRoleBinding cluster-admin│ k8s   │ rbac.yaml     │ 4d  ›  │
│ 🟠 HGH│ SQL query by string concat     │ code   │ db/user.go:42 │ 4d  ›  │
│ 🟠 HGH│ lodash 4.17.15 — CVE-2021-23337│ deps   │ package.json  │ 12d ›  │
│ 🟡 MED│ Missing Content-Security-Policy│ pent   │ :443 /        │ 1d  ›  │
│ ⬜ SUP│ ~~Weak hash MD5~~ (suppressed) │ code   │ util/etag.go  │ 9d  ›  │
├──────────────────────────────────────────────────────────────────────────┤
│                    ‹ 1  [2]  3 ›            25 per page ▾                │
└──────────────────────────────────────────────────────────────────────────┘
```

| Element | Detail |
|---|---|
| Filters | Severity, engine, status, CWE multiselect + free text. **All reflected in the URL** |
| Active filter chips | Individually removable, with "Clear all" |
| Table | Virtualised, sticky header, sortable by severity/age/engine |
| Row | Severity badge (colour+icon+text), title, engine icon, location (monospace), age, chevron |
| Suppressed rows | Dimmed with strikethrough title — visible but visually de-emphasised (FR-SCR-007) |
| Bulk selection | Checkbox column → bulk triage (P1) |
| Row click | Opens the detail drawer; deep link still works as a full page |
| Empty | "No findings match these filters" + Clear all |

### 5.3 Screen 9 — Finding detail (hero)

**Purpose:** understand and fix, in that order.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ 🟠 HIGH · confidence: high            [Acknowledge] [Suppress] [× Close] │
│ SQL query built by string concatenation                                  │
│ codescan.injection.sql-string-concat                                     │
│ CWE-89 · A03:2021 · open · first seen 4 days ago (3 scans)               │
├──────────────────────────────────────────────────────────────────────────┤
│ WHAT THIS MEANS                                                          │
│ User-controlled input reaches a SQL query that is assembled by joining    │
│ strings together. An attacker can change the meaning of the query and     │
│ read or modify data they should not have access to.                      │
├──────────────────────────────────────────────────────────────────────────┤
│ WHERE                          internal/db/user.go:42–44                  │
│  39 │ func GetUser(name string) (*User, error) {                          │
│  40 │     db := conn()                                                    │
│  41 │                                                                     │
│▶ 42 │     q := "SELECT * FROM users WHERE name = '" + name + "'"          │
│▶ 43 │     row := db.QueryRow(q)                                           │
│  44 │     return scanUser(row)                                            │
├──────────────────────────────────────────────────────────────────────────┤
│ HOW TO FIX                                                               │
│ Use a parameterised query. Pass `name` as a bound argument rather than    │
│ concatenating it into the SQL string.                                     │
├──────────────────────────────────────────────────────────────────────────┤
│ ┃ 🤖 AI SUGGESTED PATCH        ✓ verified    [Copy] [Download .patch]    │
│ ┃ ⓘ AI-generated — review before applying                                │
│ ┃  - q := "SELECT * FROM users WHERE name = '" + name + "'"              │
│ ┃  - row := db.QueryRow(q)                                               │
│ ┃  + row := db.QueryRow("SELECT * FROM users WHERE name = $1", name)     │
├──────────────────────────────────────────────────────────────────────────┤
│ ▸ References   ▸ History (3 scans)   ▸ Rule details                      │
└──────────────────────────────────────────────────────────────────────────┘
```

**The section order is the design decision.** Plain-language impact → exact location → deterministic fix → AI patch. A developer who reads only the first two sections still knows what to do. The AI patch is an accelerator, visually separated by the purple rail and never presented as authoritative.

The purple left rail (`--ai`), the robot glyph, and the "review before applying" line are all mandatory on AI content (FR-AI-012).

### 5.4 Screen 6 — Scan progress

Live view, polling every 2 s. Seven engine cards, each showing status, an indeterminate or determinate progress bar, elapsed time, and a running finding count. Findings appear as they stream in — the scan is not a black box for four minutes. Cancel button with confirmation. `aria-live="polite"` on the status region.

### 5.5 Screen 5 — New Scan wizard

Three steps: **Target** (repository or pentest target) → **Engines** (all selected by default, individually toggleable, each with a one-line description) → **Review** (summary + estimated duration + Start). For a pentest, an additional mandatory attestation step with the full authorisation text and an explicit checkbox — the Start button is disabled until it is checked (NFR-CMP-001).

### 5.6 Screen 3 — Projects list

Card grid, hover-elevated and fully keyboard-operable (`role="button"`, `tabIndex`, Enter/Space): name, status pill, repository with its `GitHubMark` (§4.5) and default branch, a credential-status line for private repos, a per-card `ContextMenu` (Settings/Archive, real — clicking it stops the card's own click-to-open from also firing), and a relative "Created …" caption (`Intl.RelativeTimeFormat`, `lib/format.ts`). Empty state: illustration + "Create your first project". Primary action top-right (the `TopBar`'s "+ New Project"). Wrapped in the same `TopBar`/`SidebarNav` chrome as every other authenticated screen (§4.4). **Built.**

Still not on the cards: last scan time, risk score chip, verdict pill, severity mini-bar — blocked on scan data that doesn't exist until Phase 6, so the cards show what's real today rather than a mocked score.

### 5.7 Screen 10 — Pentest targets

Target list with status pills (`awaiting_attestation` · `attested` · `blocked` · `revoked`). Adding a target shows inline validation results, including the resolved IPs and any block reason in plain language ("This resolves to a private address and cannot be tested"). The attestation dialog shows the full authorisation statement with the user's name and the target — deliberately weighty, because it is a legal record.

### 5.8 Screens 1, 2 — Login and Register (built, rev 1.5)

**Direction adapted from Aikido's split-screen login.** Replaces the earlier single-dark-hero-with-floating-nav treatment. Two fixed panels, side by side, present regardless of the app's own light/dark theme mode (§2.10) — the same "brand moment, not a themed surface" rule as the Landing hero (§2.9):

- **Left panel** (`AuthShell`, `--color-auth-panel-bg`/`-fg`/`-fg-secondary`, always white/dark-text) — a small logo linking home top-left (no full `PublicNav`; a focused auth screen shouldn't compete for attention with Guides/Blog mid-signup), then a centred column: a small accent pill above the headline ("Welcome back" / "Get started free"), a serif headline, one line of subtext, the real form, and a link to the other auth screen below. `Input`/`Button` render correctly regardless of the visitor's theme preference via `.auth-panel-light` — a local re-pin of every theme-relative token to its light value, scoped to this panel only (the same mechanism `.dark` itself uses, just applied locally instead of to `<html>`), so a signed-out visitor in Dark mode never sees a dark input box floating on a fixed-white panel.
- **Right panel** (`AuthSidePanel`, `lg:` and up only) — `HeroGlow` background + three real GuardPipe claims as bordered info cards, reusing the exact copy already on the Landing page's feature cards (repository/target scanning, sandboxed execution, one explainable score). **Not included, on purpose:** Aikido's OAuth-provider buttons (GitHub/GitLab/Bitbucket — GuardPipe has no OAuth, FR-IAM-010 is a documented Stretch goal, not built) and its trust badges ("Trusted by…" customer logos, SOC 2, ISO 27001 — GuardPipe has no customers and no compliance certification; fabricating either would be exactly the kind of dishonest UI this document argues against everywhere else, §4.4/§4.5). The centre column is a real email/password form instead of provider buttons — same layout, honest content.

Rules catalogue (Screen 11) and Settings (Screen 12) are unspecified beyond the original note: rules as a filterable table with an expandable detail row, settings as a tabbed form (Profile · Security · Preferences) — neither is built yet.

### 5.9 Screens 13–17 — Public site: Landing, Blog, Guides

**Purpose:** get someone from "what is this" to "I trust this enough to sign up" in one scroll, and give existing users a place to learn the product that isn't a tooltip. This surface sits entirely outside the authenticated app shell — its own nav, its own layout, no sidebar, no severity system (§1).

**Direction:** adapted from tridentsecurity.io — a dark hero with an animated blue gradient glow behind a serif display headline, a floating pill-shaped nav, then alternating light sections of paired text-and-visual cards, closing on a call-to-action band. Adapted, not copied: GuardPipe keeps a single blue accent (`--accent`, already in the palette) rather than introducing Trident's red, reduces the "juggling" motion to a slow-looping gradient (§2.9, `prefers-reduced-motion`-safe), and replaces Trident's stock-photo footer band with a plain gradient band — GuardPipe has no photography budget or asset, and a fabricated stock photo would be a worse choice than an honest gradient.

**Screen 13 — Landing (`/`)**

```
┌──────────────────────────────────────────────────────────────────────────┐
│   ψ GuardPipe        Guides   Blog              [Sign in]  [Get Started] │  ← floating pill nav, sticky
├──────────────────────────────────────────────────────────────────────────┤
│                         (animated blue gradient glow)                    │
│                    One score for your whole supply chain.                │  ← serif, text-display-hero
│         Seven SDLC stages, one explainable 0–100 risk verdict,           │  ← sans, text-body
│              AI-authored fixes. Built to be attacked and survive it.     │
│                          [ Get Started free ]                            │
├──────────────────────────────────────────────────────────────────────────┤
│  Docs → Code → Deps → Containers → K8s → CI/CD → Pentest   (icon strip)  │  ← the 7 engines, mirrors Trident's logo row
├──────────────────────────────────────────────────────────────────────────┤
│  ┌─ One score, not seven reports ──────┐  ┌─ Every finding, explained ─┐ │
│  │ [ RiskGauge + pipeline mock ]        │  │ [ finding detail mock ]     │ │
│  └───────────────────────────────────────┘  └─────────────────────────┘ │
│  ┌─ Sandboxed by default ──────────────┐                                 │
│  │ Pentest runs in a no-network,        │                                 │
│  │ read-only, non-root container.       │                                 │
│  └───────────────────────────────────────┘                                │
├──────────────────────────────────────────────────────────────────────────┤
│                (gradient CTA band) See your risk before it ships.        │
│                          [ Get Started free ]                            │
├──────────────────────────────────────────────────────────────────────────┤
│ Product        Guides         Blog          Legal                        │  ← footer nav columns
└──────────────────────────────────────────────────────────────────────────┘
```

| Element | Detail |
|---|---|
| Nav | `PublicNav`, floating, condenses on scroll; unauthenticated → "Sign in" + "Get Started"; authenticated → "Dashboard" instead |
| Hero | `HeroGlow` background, serif headline, one-sentence sans subhead, single primary CTA — no secondary CTA competing for attention |
| Engine strip | The 7 `EngineIcon`s already built for the product, reused here — GuardPipe's actual differentiators standing in for Trident's cloud-provider logo row |
| Feature cards | 3 `MarketingCard`s, each pairing one sentence of claim with a small real (or realistic mock) screenshot of the actual product — a security buyer trusts a real UI over an illustration |
| CTA band | `MarketingCtaBand`, gradient not photo |
| Footer | Nav columns + legal, dark background |

**Screen 14 — Blog index (`/blog`)** — card grid using `BlogCard`. Purpose: "how to use this stuff," written as short, practical posts, not a general security-research blog (that's out of scope for a 4-week project) — e.g. *"Reading your first risk score," "Connecting a private repository," "What 'partial scan' means and when to trust it."* Suggested categories: **Getting Started · Engine Deep Dives · Release Notes**. Eyebrow label uses `--accent`, never a severity colour — this is content, not a finding.

**Screen 15 — Blog post (`/blog/:slug`)** — eyebrow, serif H1, byline + date + read time, prose body capped at ~68 characters per line for readability, `CodeBlock` for any snippets, closing CTA band linking back to Guides or Sign up.

**Screen 16 — Guides index (`/guides`)** — 3-column `GuideCard` grid, one card per major workflow area (mirrors Trident's 3-up "AI penetration testing / Cloud attack path analysis / Continuous penetration testing" layout): e.g. **Running your first scan · Reading your risk score and verdict · Connecting a repository and pentest target**. This is the promoted, permanent version of the in-product help content — the guide someone reads *before* they've logged in, to decide whether to.

**Screen 17 — Guide detail (`/guides/:slug`)** — two-column docs layout: `GuideSidebar` (sticky, lists the guide's own sections) + prose content. Serif for the guide title only, sans for everything else — a guide is closer to documentation than to marketing copy, so it reads calmer than the Landing page despite sharing its nav and footer.

**Content is static.** Blog posts and guides are Markdown/MDX files bundled into the frontend build (e.g. via Vite's raw/glob import or a small MDX plugin) — no CMS, no new backend module, no database table. This keeps the public site inside the zero-budget, 4-week constraint: it is presentation, not a new feature surface for the backend to serve.

---

## 6. Interaction patterns

| Pattern | Specification |
|---|---|
| Loading | Skeletons that match the final layout. Spinners only inside buttons |
| Optimistic triage | Status changes apply instantly and roll back visibly on failure |
| Destructive confirmation | Delete project / cancel scan require a modal naming the exact object (NFR-USE-004) |
| Toasts | Success 4 s auto-dismiss, top-right; errors persist until dismissed |
| Deep linking | Every filtered view and every finding has a shareable URL |
| Keyboard | `⌘K` command palette · `/` search · `j`/`k` row navigation · `Esc` close · `?` shortcuts |
| Long lists | Virtualised, sticky header, page-size selector |
| Copy actions | Every code block, path, hash, and diff has a copy button with a confirmation tick |

---

## 7. Content and voice

| Rule | Bad | Good |
|---|---|---|
| Impact before taxonomy | "CWE-89 detected" | "An attacker could read your database" |
| Actionable errors | "Error 500" | "The scan could not reach the repository. Check the access token." |
| Empty states offer an action | "No data" | "No scans yet — run your first scan to see findings" |
| No blame | "You made a mistake" | "This query is built by string concatenation" |
| Numbers with meaning | "68" | "68 / 100 — Block" |
| Honest AI attribution | (unlabelled) | "AI-generated — review before applying" |

**Severity labels are always spelled out** — `CRITICAL`, `HIGH`, never `C`/`H`, and never a bare coloured dot.

---

## 8. Accessibility checklist (design-side)

- [ ] Every colour pair verified ≥ 4.5:1 (text) / ≥ 3:1 (UI, large text)
- [ ] Every severity treatment carries an icon and a text label
- [ ] Focus states designed for every interactive component (2 px `--accent` ring, 2 px offset)
- [ ] Touch/click targets ≥ 44 × 44 px
- [ ] No information conveyed by colour, position, or shape alone
- [ ] Charts have accessible text equivalents specified
- [ ] Designs verified under a deuteranopia and a protanopia filter
- [ ] Reduced-motion variants specified for all animated elements
- [ ] Dark mode contrast verified independently — not assumed from light mode
- [ ] Theme mode (§2.10): Light, Dark, and System all verified with no flash-of-wrong-theme on a hard reload, in at least one Chromium and one non-Chromium browser
- [ ] `TopBar`/`SidebarNav`/`ProjectTabBar`/overlay panels (§4.4) all keyboard-navigable — a mouse-only chrome spec is not accessible chrome

---

## 9. Handoff to code

| Design artefact | Code artefact |
|---|---|
| Figma Variable `sev/critical` | CSS `--sev-critical` → Tailwind `text-severity-critical` |
| Component `Badge/Severity/Critical` | `<SeverityBadge severity="critical" />` |
| Frame `Screen — Findings Explorer` | `pages/FindingsExplorerPage.tsx` |
| Pattern `State/Empty/NoFindings` | `<EmptyState variant="no-findings" />` |

**Process**
1. M6 publishes Foundations (page `01`) → M5 encodes tokens in `globals.css`. **This happens in Sprint 0, before any screen is designed** — it unblocks all frontend work.
2. M6 designs wireframes for all 12 screens → team review → hi-fi for the three hero screens.
3. M5 builds against wireframes; hi-fi refines styling, not structure.
4. Handoff page carries redlines and the token map. Anything ambiguous is resolved in the doc, not in Figma comments.

**Rule:** if the design and this document disagree, this document wins and Figma gets updated. One source of truth, and it is the one under version control.

---

## 10. Design deliverable checklist

- [ ] Figma file created with all 10 pages (including `09 · Public Site`)
- [ ] All tokens published as Figma Variables with light + dark modes, plus the public-site-only tokens (§2.9) in their own variable collection
- [ ] All primitives with full variant coverage
- [ ] All 24 domain components (18 product + 6 public-site, §4.2–4.3)
- [ ] All 9 Jira-inspired app-shell components (§4.4: `TopBar`, `SidebarNav`, `ProjectTabBar`, `UserMenu`, `ThemeSubmenu`, `NotificationPanel`, `GlobalSearch`, `ContextMenu`, `MetricCard`) — **built in code (rev 1.3), not yet in Figma** — code shipped ahead of the design file this time; back-fill the Figma components from the shipped UI rather than the other way around
- [ ] Theme-mode switcher (§2.10) designed for all three states and specified for the no-flash boot behaviour — **built in code, not yet in Figma**
- [ ] 17 wireframes (12 product screens + 5 public-site screens)
- [ ] 3 hero screens in high fidelity (desktop) — plus the Landing page in high fidelity, since it is what a demo audience or a checkpoint reviewer sees first
- [ ] 4 primary screens at tablet width
- [ ] Loading / empty / error / partial patterns for every data view
- [ ] Clickable prototype covering the demo path
- [ ] Accessibility checklist (§8) complete
- [ ] Handoff page with the token map
- [ ] Figma link added to [README](README.md) and the project charter
