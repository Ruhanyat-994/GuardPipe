import type { ReactElement } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'

/**
 * Protected-route layout per documentation/08-frontend-architecture.md §5:
 * redirects to /login?returnTo=<path> when unauthenticated.
 *
 * While `isInitializing` is true, App's bootstrap() call is still trying a
 * silent refresh from the httpOnly cookie — redirecting immediately would
 * flash the login page on every hard reload even for a user with a valid
 * session, so this waits rather than deciding early.
 */
export function RequireAuth({ children }: { children: ReactElement }): ReactElement | null {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const isInitializing = useAuthStore((s) => s.isInitializing)
  const location = useLocation()

  if (isInitializing) {
    return null
  }

  if (!isAuthenticated) {
    const returnTo = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?returnTo=${returnTo}`} replace />
  }

  return children
}
