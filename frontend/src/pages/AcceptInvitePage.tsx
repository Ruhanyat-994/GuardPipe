import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { CheckCircle2, XCircle } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Button } from '../components/ui/Button'
import { ApiError } from '../lib/apiClient'
import { acceptInvite } from '../lib/organizationApi'
import { useAuthStore } from '../stores/authStore'

/**
 * `/invites/:token/accept` — BUILD_GUIDE.md Phase 15's invite-accept
 * landing page. There is no email delivery in this build (zero-budget, no
 * mail adapter), so an org admin shares this URL directly (copied from the
 * Members tab's "invite created" confirmation, lib/organizationApi.ts's own
 * doc comment) rather than it arriving in an inbox.
 *
 * If the visitor isn't logged in yet, they're sent to /login (or /register,
 * for a brand-new invitee — organization.Service.AcceptInvite's own doc
 * comment names this exact "register or log in, then accept" two-call
 * flow) with `returnTo` preserved, so control lands back here once
 * authenticated; the invited email must match whichever account they log
 * into, or the backend rejects it with a clear, actionable message.
 */
export function AcceptInvitePage() {
  const { token } = useParams<{ token: string }>()
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const isInitializing = useAuthStore((s) => s.isInitializing)
  const navigate = useNavigate()

  const [status, setStatus] = useState<'accepting' | 'accepted' | 'error'>('accepting')
  const [error, setError] = useState<string | null>(null)
  // Guards against firing the accept call twice (e.g. React 18 Strict
  // Mode's double-invoke in dev) — a ref, not state, since it's not
  // something a render needs to react to.
  const startedRef = useRef(false)

  useEffect(() => {
    if (isInitializing || !token) return
    if (!isAuthenticated) return // the CTA below handles this state instead
    if (startedRef.current) return
    startedRef.current = true

    acceptInvite(token)
      .then(() => setStatus('accepted'))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not accept this invite.')
        setStatus('error')
      })
  }, [isAuthenticated, isInitializing, token])

  const returnTo = encodeURIComponent(`/invites/${token ?? ''}/accept`)

  return (
    <AuthShell>
      <h1 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
        Join an organization
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
            You&rsquo;ve joined the organization.
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
