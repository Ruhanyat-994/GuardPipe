import { useEffect } from 'react'
import { Route, Routes } from 'react-router-dom'
import { LandingPage } from './pages/LandingPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { DashboardPage } from './pages/DashboardPage'
import { GuidesIndexPage } from './pages/GuidesIndexPage'
import { GuideDetailPage } from './pages/GuideDetailPage'
import { BlogIndexPage } from './pages/BlogIndexPage'
import { BlogPostPage } from './pages/BlogPostPage'
import { PlaceholderPage } from './pages/PlaceholderPage'
import { RequireAuth } from './components/RequireAuth'
import { useAuthStore } from './stores/authStore'

/**
 * Route composition only — no business logic here, matching the "Pages"
 * layer rule in documentation/08-frontend-architecture.md §2. Routes match
 * documentation/08-frontend-architecture.md §5.
 */
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
      <Route
        path="/projects"
        element={
          <RequireAuth>
            <DashboardPage />
          </RequireAuth>
        }
      />
      <Route path="*" element={<PlaceholderPage title="404 — not found" phase="—" />} />
    </Routes>
  )
}

export default App
