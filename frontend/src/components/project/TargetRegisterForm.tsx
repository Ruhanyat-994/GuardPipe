import { type FormEvent, useState } from 'react'
import { Crosshair } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { Input } from '../ui/Input'
import { ApiError } from '../../lib/apiClient'
import { registerTarget, type Target } from '../../lib/projectsApi'

/**
 * FR-PRJ-006/007: registers and DNS-validates a pentest target. The
 * ownership-attestation gate itself is a Phase 7 (pentest engine) UI —
 * this just proves registration/validation works end to end. Shared
 * between `ProjectCreatePage` and `ProjectTargetsPage`.
 */
export function TargetRegisterForm({
  projectId,
  onRegistered,
}: {
  projectId: string
  onRegistered?: (target: Target) => void
}) {
  const [target, setTarget] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [registered, setRegistered] = useState<Target | null>(null)

  async function handleRegister(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const t = await registerTarget(projectId, target)
      setRegistered(t)
      setTarget('')
      onRegistered?.(t)
    } catch (err) {
      setError(
        err instanceof ApiError ? err.problem.detail : 'Something went wrong. Please try again.',
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card>
      <div className="flex items-center gap-2">
        <Crosshair className="h-4 w-4 text-text-tertiary" aria-hidden="true" />
        <CardTitle className="text-h3">Register a pentest target</CardTitle>
      </div>
      <CardDescription className="mt-1">
        A host or URL GuardPipe may later probe, once you accept the authorisation attestation.
      </CardDescription>

      <form onSubmit={handleRegister} className="mt-4 flex flex-col gap-4">
        <div>
          <label htmlFor="target" className="mb-1 block text-body-sm text-text-secondary">
            Host or URL
          </label>
          <Input
            id="target"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="staging.acme.example"
          />
        </div>

        {error && (
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        )}
        {registered && (
          <p className="text-body-sm text-success">
            Registered {registered.normalized_host} — status: {registered.status.replace('_', ' ')}.
          </p>
        )}

        <Button
          type="submit"
          variant="secondary"
          loading={submitting}
          disabled={!target}
          className="self-start"
        >
          Register target
        </Button>
      </form>
    </Card>
  )
}
