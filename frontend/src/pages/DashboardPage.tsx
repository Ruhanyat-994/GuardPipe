import { useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { useAuthStore } from '../stores/authStore'

/**
 * The "still-empty dashboard" Phase 2's Done-when criterion asks for
 * (BUILD_GUIDE.md) — real project/scan content lands in Phase 3 onward.
 * What matters here is that it's real: the user object comes from the
 * authenticated /auth/me call, not a mock.
 */
export function DashboardPage() {
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <main className="mx-auto max-w-2xl px-8 py-16">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-display text-text-primary">GuardPipe</h1>
        <Button variant="secondary" onClick={handleLogout}>
          Sign out
        </Button>
      </div>

      <Card>
        <CardTitle>Welcome{user ? `, ${user.displayName}` : ''}.</CardTitle>
        <CardDescription className="mt-2">
          {user?.email} · role: {user?.role}
        </CardDescription>
        <CardDescription className="mt-4">
          There are no projects yet — project creation lands in Phase 3.
        </CardDescription>
      </Card>
    </main>
  )
}
