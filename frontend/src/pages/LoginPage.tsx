import { type FormEvent, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Button } from '../components/ui/Button'
import { Input } from '../components/ui/Input'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ApiError } from '../lib/apiClient'
import { useAuthStore } from '../stores/authStore'

/**
 * documentation/09-ui-ux-design-system.md §5.8: "centred auth card with the
 * product mark." Wired to the real `/auth/login` endpoint — no mock data.
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
    <main className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardTitle>Sign in to GuardPipe</CardTitle>
        <CardDescription className="mt-1 mb-6">
          Don&apos;t have an account?{' '}
          <Link to="/register" className="text-accent underline">
            Register
          </Link>
        </CardDescription>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label htmlFor="email" className="mb-1 block text-body-sm text-text-secondary">
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
            <label htmlFor="password" className="mb-1 block text-body-sm text-text-secondary">
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

          <Button type="submit" loading={submitting} className="mt-2">
            Sign in
          </Button>
        </form>
      </Card>
    </main>
  )
}
