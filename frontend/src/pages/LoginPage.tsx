import { type FormEvent, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { Button } from '../components/ui/Button'
import { Input } from '../components/ui/Input'
import { ApiError } from '../lib/apiClient'
import { useAuthStore } from '../stores/authStore'

/**
 * documentation/09-ui-ux-design-system.md §5.8, split-screen direction
 * adapted from Aikido (§4.4-adjacent — see `AuthShell`). Wired to the real
 * `/auth/login` endpoint — no mock data, no OAuth (not built, FR-IAM-010
 * is a documented Stretch goal).
 */
export function LoginPage() {
  const login = useAuthStore((s) => s.login)
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(email, password)
      const returnTo = searchParams.get('returnTo')
      navigate(returnTo ? decodeURIComponent(returnTo) : '/projects', { replace: true })
    } catch (err) {
      // Same message whether the email is unknown or the password is wrong
      // — the backend already guarantees no user-enumeration
      // (documentation/07-api-specification.md §2), the frontend just
      // displays whatever `detail` it was given.
      setError(
        err instanceof ApiError ? err.problem.detail : 'Something went wrong. Please try again.',
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthShell>
      <span className="mb-4 inline-flex items-center gap-1.5 rounded-full border border-accent/30 bg-accent/10 px-3 py-1 text-caption font-semibold text-accent">
        Welcome back
      </span>
      <h1 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
        Sign in to GuardPipe.
      </h1>
      <p className="mt-2 mb-8 text-body-sm text-auth-panel-fg-secondary">
        Continue to your dashboard and pick up where you left off.
      </p>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div>
          <label htmlFor="email" className="mb-1 block text-body-sm text-auth-panel-fg-secondary">
            Email
          </label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div>
          <label
            htmlFor="password"
            className="mb-1 block text-body-sm text-auth-panel-fg-secondary"
          >
            Password
          </label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>

        {error && (
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        )}

        <Button type="submit" size="lg" loading={submitting} className="mt-2 w-full">
          Sign in
        </Button>
      </form>

      <p className="mt-6 text-center text-body-sm text-auth-panel-fg-secondary">
        Don&apos;t have an account?{' '}
        <Link to="/register" className="font-medium text-accent hover:underline">
          Sign up free
        </Link>
      </p>
    </AuthShell>
  )
}
