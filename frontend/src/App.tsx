import { Route, Routes } from 'react-router-dom'
import { LandingPage } from './pages/LandingPage'
import { PlaceholderPage } from './pages/PlaceholderPage'
import { RequireAuth } from './components/RequireAuth'

/**
 * Route composition only — no business logic here, matching the "Pages"
 * layer rule in documentation/08-frontend-architecture.md §2. Routes match
 * documentation/08-frontend-architecture.md §5 exactly; most render
 * PlaceholderPage until their owning phase builds the real screen.
 */
function App() {
  return (
    <Routes>
      <Route path="/" element={<LandingPage />} />
      <Route
        path="/blog"
        element={<PlaceholderPage title="Blog" phase="Phase 9 (public site)" />}
      />
      <Route
        path="/blog/:slug"
        element={<PlaceholderPage title="Blog post" phase="Phase 9 (public site)" />}
      />
      <Route
        path="/guides"
        element={<PlaceholderPage title="Guides" phase="Phase 9 (public site)" />}
      />
      <Route
        path="/guides/:slug"
        element={<PlaceholderPage title="Guide detail" phase="Phase 9 (public site)" />}
      />
      <Route path="/login" element={<PlaceholderPage title="Login" phase="Phase 2 (identity)" />} />
      <Route
        path="/register"
        element={<PlaceholderPage title="Register" phase="Phase 2 (identity)" />}
      />
      <Route
        path="/projects"
        element={
          <RequireAuth>
            <PlaceholderPage title="Projects" phase="Phase 3 (projects)" />
          </RequireAuth>
        }
      />
      <Route path="*" element={<PlaceholderPage title="404 — not found" phase="—" />} />
    </Routes>
  )
}

export default App
