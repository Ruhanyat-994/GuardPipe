import type { ReactElement } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'

/**
 * Protected-route layout per documentation/08-frontend-architecture.md §5:
 * redirects to /login?returnTo=<path> when unauthenticated. There is
 * nothing behind this gate yet (Phase 2 adds the identity module this
 * checks against) — this is the wrapper Phase 2's protected pages mount
 * under.
 */
export function RequireAuth({ children }: { children: ReactElement }): ReactElement {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const location = useLocation()

  if (!isAuthenticated) {
    const returnTo = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?returnTo=${returnTo}`} replace />
  }

  return children
}
