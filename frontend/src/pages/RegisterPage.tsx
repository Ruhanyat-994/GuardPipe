import { type FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { Button } from '../components/ui/Button'
import { Input } from '../components/ui/Input'
import { ApiError } from '../lib/apiClient'
import { useAuthStore } from '../stores/authStore'

/**
 * documentation/09-ui-ux-design-system.md §5.8, split-screen direction
 * adapted from Aikido (see `AuthShell`). No OAuth — a real email/password
 * form in the same layout instead.
 */
export function RegisterPage() {
  const register = useAuthStore((s) => s.register)
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFieldErrors({})
    setFormError(null)
    setSubmitting(true)
    try {
      await register(email, displayName, password)
      navigate('/login', { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.problem.errors && err.problem.errors.length > 0) {
          setFieldErrors(Object.fromEntries(err.problem.errors.map((fe) => [fe.field, fe.message])))
        } else {
          setFormError(err.problem.detail)
        }
      } else {
        setFormError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthShell>
      <span className="mb-4 inline-flex items-center gap-1.5 rounded-full border border-accent/30 bg-accent/10 px-3 py-1 text-caption font-semibold text-accent">
        Get started free
      </span>
      <h1 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
        Create your account.
      </h1>
      <p className="mt-2 mb-8 text-body-sm text-auth-panel-fg-secondary">
        No credit card, no OAuth — just an email and a password to start scanning.
      </p>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div>
          <label
            htmlFor="display_name"
            className="mb-1 block text-body-sm text-auth-panel-fg-secondary"
          >
            Display name
          </label>
          <Input
            id="display_name"
            autoComplete="name"
            required
            invalid={Boolean(fieldErrors.display_name)}
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
          />
          {fieldErrors.display_name && <FieldError message={fieldErrors.display_name} />}
        </div>
        <div>
          <label htmlFor="email" className="mb-1 block text-body-sm text-auth-panel-fg-secondary">
            Email
          </label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            required
            invalid={Boolean(fieldErrors.email)}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          {fieldErrors.email && <FieldError message={fieldErrors.email} />}
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
            autoComplete="new-password"
            minLength={12}
            required
            invalid={Boolean(fieldErrors.password)}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {fieldErrors.password ? (
            <FieldError message={fieldErrors.password} />
          ) : (
            <p className="mt-1 text-caption text-auth-panel-fg-secondary">
              At least 12 characters.
            </p>
          )}
        </div>

        {formError && (
          <p role="alert" className="text-body-sm text-danger">
            {formError}
          </p>
        )}

        <Button type="submit" size="lg" loading={submitting} className="mt-2 w-full">
          Create account
        </Button>
      </form>

      <p className="mt-6 text-center text-body-sm text-auth-panel-fg-secondary">
        Already have an account?{' '}
        <Link to="/login" className="font-medium text-accent hover:underline">
          Sign in
        </Link>
      </p>
    </AuthShell>
  )
}

function FieldError({ message }: { message: string }) {
  return (
    <p role="alert" className="mt-1 text-caption text-danger">
      {message}
    </p>
  )
}
