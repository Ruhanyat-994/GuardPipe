import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { CheckCircle2, XCircle } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Button } from '../components/ui/Button'
import { ApiError } from '../lib/apiClient'
import { acceptProjectInvite } from '../lib/projectsApi'
import { useAuthStore } from '../stores/authStore'

/**
 * `/project-invites/:token/accept` — the project-collaborators follow-up's
 * counterpart to AcceptInvitePage.tsx, for the same reason that one exists:
 * no email delivery in this build, so a project owner shares this URL
 * directly (from ProjectCollaboratorsSection's "invite sent" state) rather
 * than it arriving in an inbox. The in-app notification feed
 * (NotificationPanel.tsx) is the primary flow for an already-registered
 * invitee; this page is the fallback for someone not logged in yet when the
 * link arrives.
 */
export function AcceptProjectInvitePage() {
  const { token } = useParams<{ token: string }>()
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const isInitializing = useAuthStore((s) => s.isInitializing)
  const navigate = useNavigate()

  const [status, setStatus] = useState<'accepting' | 'accepted' | 'error'>('accepting')
  const [error, setError] = useState<string | null>(null)
  const startedRef = useRef(false)

  useEffect(() => {
    if (isInitializing || !token) return
    if (!isAuthenticated) return
    if (startedRef.current) return
    startedRef.current = true

    acceptProjectInvite(token)
      .then(() => setStatus('accepted'))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not accept this invite.')
        setStatus('error')
      })
  }, [isAuthenticated, isInitializing, token])

  const returnTo = encodeURIComponent(`/project-invites/${token ?? ''}/accept`)

  return (
    <AuthShell>
      <h1 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
        Join a shared project
      </h1>

      {isInitializing ? (
        <p className="mt-4 text-body-sm text-auth-panel-fg-secondary">Checking your invite…</p>
      ) : !isAuthenticated ? (
        <div className="mt-6">
          <p className="text-body-sm text-auth-panel-fg-secondary">
            Sign in or create an account with the email address this invite was sent to, then come
            back here to finish joining.
          </p>
          <div className="mt-4 flex gap-3">
            <Button onClick={() => navigate(`/login?returnTo=${returnTo}`)}>Sign in</Button>
            <Button variant="secondary" onClick={() => navigate(`/register?returnTo=${returnTo}`)}>
              Create account
            </Button>
          </div>
        </div>
      ) : status === 'accepting' ? (
        <p className="mt-4 text-body-sm text-auth-panel-fg-secondary">Accepting your invite…</p>
      ) : status === 'accepted' ? (
        <div className="mt-6">
          <p className="flex items-center gap-2 text-body-sm text-success">
            <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
            You now have access to this project. It won&rsquo;t show up with the rest of your
            dashboard until you switch to it from the account menu.
          </p>
          <Button className="mt-4" onClick={() => navigate('/dashboard')}>
            Go to dashboard
          </Button>
        </div>
      ) : status === 'error' ? (
        <div className="mt-6">
          <p role="alert" className="flex items-center gap-2 text-body-sm text-danger">
            <XCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
            {error}
          </p>
          <Link
            to="/dashboard"
            className="mt-4 inline-block text-body-sm text-accent hover:underline"
          >
            Go to dashboard instead
          </Link>
        </div>
      ) : null}
    </AuthShell>
  )
}
