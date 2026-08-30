import type { ReactElement } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'

/**
 * Gate for the `/admin` route tree (BUILD_GUIDE.md Phase 14) — distinct
 * from RequireAuth, which this sits behind, not instead of. A non-operator
 * (including an org's own `role: 'admin'`) is redirected to `/dashboard`
 * rather than shown a 403 page — the nav entry is already hidden for them
 * (AppShell/UserMenu), so landing here at all means a stale bookmark or a
 * guess, not a real workflow to explain an error for.
 *
 * This is UX only. The real enforcement is server-side
 * (RequirePlatformOperator, internal/transport/http/middleware) — every
 * request this shell makes is checked again there regardless of what this
 * component decides.
 */
export function RequireOperator({ children }: { children: ReactElement }): ReactElement {
  const isPlatformOperator = useAuthStore((s) => s.user?.isPlatformOperator ?? false)

  if (!isPlatformOperator) {
    return <Navigate to="/dashboard" replace />
  }

  return children
}
