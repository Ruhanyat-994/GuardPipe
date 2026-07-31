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
import { ProjectSettingsPage } from './pages/ProjectSettingsPage'
import { GuidesIndexPage } from './pages/GuidesIndexPage'
import { GuideDetailPage } from './pages/GuideDetailPage'
import { BlogIndexPage } from './pages/BlogIndexPage'
import { BlogPostPage } from './pages/BlogPostPage'
import { PlaceholderPage } from './pages/PlaceholderPage'
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
            dashboard preview; Scans/Findings have no real data source
            until Phase 6/8 so they stay PlaceholderPage; Targets/Settings
            are real, backed by the Phase 3 project API. */}
        <Route path="/projects/:id" element={<ProjectLayout />}>
          <Route index element={<DashboardPage />} />
          <Route path="scans" element={<PlaceholderPage title="Scans" phase="Phase 6" />} />
          <Route path="findings" element={<PlaceholderPage title="Findings" phase="Phase 8" />} />
          <Route path="targets" element={<ProjectTargetsPage />} />
          <Route path="settings" element={<ProjectSettingsPage />} />
        </Route>

        {/* Sidebar destinations with no project scope and no real screen
            yet — wired so the authenticated app is fully click-through-able
            for a demo (BUILD_GUIDE.md Phase 3). Each is replaced by its own
            phase. */}
        <Route path="/scans" element={<PlaceholderPage title="Scans" phase="Phase 6" />} />
        <Route path="/findings" element={<PlaceholderPage title="Findings" phase="Phase 8" />} />
        <Route
          path="/targets"
          element={<PlaceholderPage title="Pentest Targets" phase="Phase 7" />}
        />
        <Route path="/rules" element={<PlaceholderPage title="Rules" phase="Phase 7" />} />
        <Route path="/settings" element={<PlaceholderPage title="Settings" phase="Phase 9" />} />
      </Route>

      <Route path="*" element={<PlaceholderPage title="404 — not found" phase="—" />} />
    </Routes>
  )
}

export default App
