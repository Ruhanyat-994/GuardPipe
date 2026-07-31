import { type FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { Button } from '../components/ui/Button'
import { Input } from '../components/ui/Input'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ApiError } from '../lib/apiClient'
import { useAuthStore } from '../stores/authStore'

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
      <div className="mb-6 text-center">
        <h1 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
          Join GuardPipe.
        </h1>
        <p className="mt-2 text-body-sm text-white/70">
          One score for your whole supply chain, starting with your first scan.
        </p>
      </div>

      <Card>
        <CardTitle>Create your GuardPipe account</CardTitle>
        <CardDescription className="mt-1 mb-6">
          Already have an account?{' '}
          <Link to="/login" className="text-accent underline">
            Sign in
          </Link>
        </CardDescription>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label htmlFor="display_name" className="mb-1 block text-body-sm text-text-secondary">
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
            <label htmlFor="email" className="mb-1 block text-body-sm text-text-secondary">
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
            <label htmlFor="password" className="mb-1 block text-body-sm text-text-secondary">
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
              <p className="mt-1 text-caption text-text-tertiary">At least 12 characters.</p>
            )}
          </div>

          {formError && (
            <p role="alert" className="text-body-sm text-danger">
              {formError}
            </p>
          )}

          <Button type="submit" loading={submitting} className="mt-2">
            Create account
          </Button>
        </form>
      </Card>
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
