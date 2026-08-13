import { useEffect } from 'react'
import { Outlet, Route, Routes } from 'react-router-dom'
import { LandingPage } from './pages/LandingPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
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
import { RequireAuth } from './components/RequireAuth'
import { AppShell } from './components/AppShell'
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

      <Route element={<ProtectedShell />}>
        <Route path="/projects" element={<ProjectsListPage />} />
        <Route path="/projects/new" element={<ProjectCreatePage />} />

        {/* Per-project tab bar (documentation/09-ui-ux-design-system.md
            §4.4's ProjectTabBar) — Overview is the existing Phase 2/3
            dashboard preview; Scans is real (Phase 6, depscan only);
            Findings shows the most recent scan's findings inline (the full
            cross-scan explorer with filtering/triage is still Phase 8's);
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
        <Route path="/settings" element={<PlaceholderPage title="Settings" phase="Phase 9" />} />
      </Route>

      <Route path="*" element={<PlaceholderPage title="404 — not found" phase="—" />} />
    </Routes>
  )
}

export default App
