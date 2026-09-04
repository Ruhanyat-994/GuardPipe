import { useEffect } from 'react'
import { Outlet, Route, Routes } from 'react-router-dom'
import { LandingPage } from './pages/LandingPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { GlobalDashboardPage } from './pages/GlobalDashboardPage'
import { DashboardPage } from './pages/DashboardPage'
import { ProjectsListPage } from './pages/ProjectsListPage'
import { ProjectCreatePage } from './pages/ProjectCreatePage'
import { ProjectLayout } from './pages/ProjectLayout'
import { ProjectTargetsPage } from './pages/ProjectTargetsPage'
import { ProjectScansPage } from './pages/ProjectScansPage'
import { ProjectFindingsPage } from './pages/ProjectFindingsPage'
import { GlobalScansPage } from './pages/GlobalScansPage'
import { GlobalFindingsPage } from './pages/GlobalFindingsPage'
import { ScanDetailPage } from './pages/ScanDetailPage'
import { ProjectSettingsPage } from './pages/ProjectSettingsPage'
import { GuidesIndexPage } from './pages/GuidesIndexPage'
import { GuideDetailPage } from './pages/GuideDetailPage'
import { BlogIndexPage } from './pages/BlogIndexPage'
import { BlogPostPage } from './pages/BlogPostPage'
import { PlaceholderPage } from './pages/PlaceholderPage'
import { RulesPage } from './pages/RulesPage'
import { OrgSettingsPage } from './pages/OrgSettingsPage'
import { TeamDashboardPage } from './pages/TeamDashboardPage'
import { AcceptInvitePage } from './pages/AcceptInvitePage'
import { AdminOrganizationsPage } from './pages/AdminOrganizationsPage'
import { AdminOrganizationDetailPage } from './pages/AdminOrganizationDetailPage'
import { AdminPentestFlagsPage } from './pages/AdminPentestFlagsPage'
import { AdminAuditLogPage } from './pages/AdminAuditLogPage'
import { AdminSystemHealthPage } from './pages/AdminSystemHealthPage'
import { RequireAuth } from './components/RequireAuth'
import { RequireOperator } from './components/RequireOperator'
import { AppShell } from './components/AppShell'
import { AdminShell } from './components/AdminShell'
import { useAuthStore } from './stores/authStore'

/**
 * Route composition only — no business logic here, matching the "Pages"
 * layer rule in documentation/08-frontend-architecture.md §2. Routes match
 * documentation/08-frontend-architecture.md §5.
 */

/** Layout route: every authenticated screen renders inside `AppShell`
 * (documentation/09-ui-ux-design-system.md §4.4's `TopBar`/`SidebarNav`),
 * behind `RequireAuth`. A layout route (no `path`, just an `element` with
 * an `<Outlet/>`) is the idiomatic way to share this wrapper across many
 * routes without repeating it per `<Route>`. */
function ProtectedShell() {
  return (
    <RequireAuth>
      <AppShell>
        <Outlet />
      </AppShell>
    </RequireAuth>
  )
}

/** The platform-operator control plane's own layout route (BUILD_GUIDE.md
 * Phase 14) — RequireAuth, then RequireOperator, then AdminShell (never
 * AppShell) — see AdminShell's own doc comment for why this is a separate
 * shell rather than a section of the tenant-facing one. */
function ProtectedAdminShell() {
  return (
    <RequireAuth>
      <RequireOperator>
        <AdminShell>
          <Outlet />
        </AdminShell>
      </RequireOperator>
    </RequireAuth>
  )
}

function App() {
  const bootstrap = useAuthStore((s) => s.bootstrap)

  // Attempts a silent refresh from the httpOnly cookie once, on load, so a
  // hard reload doesn't force a re-login while the refresh token is still
  // valid (the access token itself never survives a reload — it's memory
  // only, documentation/07-api-specification.md §2).
  useEffect(() => {
    void bootstrap()
  }, [bootstrap])

  return (
    <Routes>
      <Route path="/" element={<LandingPage />} />
      <Route path="/blog" element={<BlogIndexPage />} />
      <Route path="/blog/:slug" element={<BlogPostPage />} />
      <Route path="/guides" element={<GuidesIndexPage />} />
      <Route path="/guides/:slug" element={<GuideDetailPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      {/* Public-ish (BUILD_GUIDE.md Phase 15) — the page itself handles both
          "already logged in" (accepts immediately) and "not logged in yet"
          (CTA to /login or /register with the token preserved), so it isn't
          wrapped in RequireAuth the way every other authenticated route is. */}
      <Route path="/invites/:token/accept" element={<AcceptInvitePage />} />

      <Route element={<ProtectedShell />}>
        {/* The org-wide landing page after login (Phase 13, partial —
            documentation/09-ui-ux-design-system.md's dashboard redesign
            note) — real data across every project's latest scan, not the
            old hardcoded per-project preview. */}
        <Route path="/dashboard" element={<GlobalDashboardPage />} />
        <Route path="/projects" element={<ProjectsListPage />} />
        <Route path="/projects/new" element={<ProjectCreatePage />} />

        {/* Per-project tab bar (documentation/09-ui-ux-design-system.md
            §4.4's ProjectTabBar) — Overview is this project's own real
            dashboard (Phase 13, partial); Scans is real (Phase 6-8);
            Findings shows the most recent scan's findings inline (the full
            cross-scan explorer with filtering/triage is still later work);
            Targets/Settings are real, backed by the Phase 3 project API. */}
        <Route path="/projects/:id" element={<ProjectLayout />}>
          <Route index element={<DashboardPage />} />
          <Route path="scans" element={<ProjectScansPage />} />
          <Route path="findings" element={<ProjectFindingsPage />} />
          <Route path="targets" element={<ProjectTargetsPage />} />
          <Route path="settings" element={<ProjectSettingsPage />} />
        </Route>

        {/* Scan detail — not project-scoped in the URL since a scan ID
            alone is enough to look it up (documentation/07-api-specification.md
            §5); reached from ProjectScansPage's "Run Scan" trigger. */}
        <Route path="/scans/:id" element={<ScanDetailPage />} />

        {/* The global sidebar destinations — Scans lets you pick any
            existing project and run it directly, plus the scan history
            across every project; Findings is the same "project -> engine ->
            findings" browser as the per-project tab, one accordion row per
            project. Targets/Settings still have no dedicated screen. */}
        <Route path="/scans" element={<GlobalScansPage />} />
        <Route path="/findings" element={<GlobalFindingsPage />} />
        <Route
          path="/targets"
          element={<PlaceholderPage title="Pentest Targets" phase="Phase 7" />}
        />
        <Route path="/rules" element={<RulesPage />} />
        {/* Team Dashboard (BUILD_GUIDE.md Phase 15) — org-wide assignment ×
            gate-verdict matrix. */}
        <Route path="/team" element={<TeamDashboardPage />} />
        {/* Org Settings → Members (BUILD_GUIDE.md Phase 15) — the previous
            Phase-9 placeholder's real content for org membership; per-account
            profile fields remain a later addition. */}
        <Route path="/settings" element={<OrgSettingsPage />} />
      </Route>

      {/* Platform admin panel (BUILD_GUIDE.md Phase 14) — a separate route
          tree with its own layout route/shell, gated by RequireOperator on
          top of RequireAuth. Not nested under ProtectedShell's <Route> above
          on purpose: it must never render inside AppShell. */}
      <Route element={<ProtectedAdminShell />}>
        <Route path="/admin/organizations" element={<AdminOrganizationsPage />} />
        <Route path="/admin/organizations/:id" element={<AdminOrganizationDetailPage />} />
        <Route path="/admin/pentest-flags" element={<AdminPentestFlagsPage />} />
        <Route path="/admin/audit-log" element={<AdminAuditLogPage />} />
        <Route path="/admin/system-health" element={<AdminSystemHealthPage />} />
      </Route>

      <Route path="*" element={<PlaceholderPage title="404 — not found" phase="—" />} />
    </Routes>
  )
}

export default App
