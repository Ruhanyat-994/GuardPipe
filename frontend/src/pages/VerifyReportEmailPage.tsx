import { useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { CheckCircle2, XCircle } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { ApiError } from '../lib/apiClient'
import { verifyReportEmail } from '../lib/notificationsApi'

/**
 * `/verify-report-email?token=…` — the link in the "confirm your report
 * address" email. Public on purpose: the token is the credential, and the
 * link is often opened on a phone with no GuardPipe session. Until this
 * succeeds, scan reports keep going to the previous address.
 */
export function VerifyReportEmailPage() {
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [status, setStatus] = useState<'verifying' | 'verified' | 'error'>(
    token ? 'verifying' : 'error',
  )
  const [email, setEmail] = useState('')
  const [error, setError] = useState(token ? '' : 'This verification link is incomplete.')
  // Strict Mode double-invokes effects in dev; the token is single-use, so
  // a second call would report "invalid" right after the first succeeded.
  const startedRef = useRef(false)

  useEffect(() => {
    if (!token || startedRef.current) return
    startedRef.current = true
    verifyReportEmail(token)
      .then((res) => {
        setEmail(res.report_email)
        setStatus('verified')
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'This link could not be verified.')
        setStatus('error')
      })
  }, [token])

  return (
    <AuthShell>
      <h1 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
        Confirm report address
      </h1>
      {status === 'verifying' && (
        <p className="mt-4 text-body-sm text-auth-panel-fg-secondary">Confirming…</p>
      )}
      {status === 'verified' && (
        <div className="mt-6">
          <p className="flex items-center gap-2 text-body-sm text-success">
            <CheckCircle2 className="h-4 w-4 shrink-0" aria-hidden="true" />
            Scan reports will now be sent to {email}.
          </p>
          <Link
            to="/settings/notifications"
            className="mt-4 inline-block text-body-sm text-accent hover:underline"
          >
            Open notification settings
          </Link>
        </div>
      )}
      {status === 'error' && (
        <div className="mt-6">
          <p role="alert" className="flex items-center gap-2 text-body-sm text-danger">
            <XCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
            {error}
          </p>
          <p className="mt-2 text-body-sm text-auth-panel-fg-secondary">
            Links work once and expire after 24 hours. You can send a new one from Settings →
            Notifications.
          </p>
          <Link
            to="/settings/notifications"
            className="mt-4 inline-block text-body-sm text-accent hover:underline"
          >
            Go to notification settings
          </Link>
        </div>
      )}
    </AuthShell>
  )
}
