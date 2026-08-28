import { useState } from 'react'
import { Button } from '../ui/Button'
import { cn } from '../../lib/cn'
import { ApiError } from '../../lib/apiClient'
import { updateFindingStatus, type ActorRef, type FindingStatus } from '../../lib/scansApi'

export interface TriageUpdate {
  status: FindingStatus
  status_reason?: string
  status_changed_by: ActorRef | null
  status_changed_at: string | null
}

const MIN_SUPPRESSION_REASON_LENGTH = 20

/** documentation/05-module-specifications.md §15's triage state machine —
 * mirrors reporting.ValidTransitions on the backend exactly (that's the
 * one enforced server-side; this table only decides which buttons to
 * show, so the two must stay in sync by hand rather than one deriving the
 * other). */
const NEXT_ACTIONS: Record<
  FindingStatus,
  { status: FindingStatus; label: string; needsReason?: boolean }[]
> = {
  open: [
    { status: 'acknowledged', label: 'Acknowledge' },
    { status: 'suppressed', label: 'Suppress', needsReason: true },
    { status: 'false_positive', label: 'Mark false positive' },
  ],
  acknowledged: [
    { status: 'fixed', label: 'Mark fixed' },
    { status: 'suppressed', label: 'Suppress', needsReason: true },
  ],
  suppressed: [{ status: 'open', label: 'Reopen' }],
  false_positive: [{ status: 'open', label: 'Reopen' }],
  fixed: [{ status: 'open', label: 'Reopen' }],
}

/**
 * The triage action row — buttons for whatever transitions are legal from
 * the finding's current status, plus the inline reason prompt suppression
 * requires (>=20 characters, FR-RPT-005). Purely local state: on success,
 * `onChanged` hands the caller the new status so it can update its own
 * view without a full re-fetch.
 */
export function TriageActions({
  findingId,
  status,
  onChanged,
}: {
  findingId: string
  status: FindingStatus
  onChanged: (update: TriageUpdate) => void
}) {
  const [pending, setPending] = useState<FindingStatus | null>(null)
  const [reasonFor, setReasonFor] = useState<FindingStatus | null>(null)
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)

  const actions = NEXT_ACTIONS[status]

  async function apply(target: FindingStatus, withReason?: string) {
    setError(null)
    setPending(target)
    try {
      const updated = await updateFindingStatus(findingId, target, withReason)
      onChanged(updated)
      setReasonFor(null)
      setReason('')
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not update this finding.')
    } finally {
      setPending(null)
    }
  }

  const reasonTooShort = reason.trim().length < MIN_SUPPRESSION_REASON_LENGTH

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        {actions.map((a) => (
          <Button
            key={a.status}
            type="button"
            variant="secondary"
            size="sm"
            loading={pending === a.status}
            disabled={pending !== null}
            onClick={() => (a.needsReason ? setReasonFor(a.status) : void apply(a.status))}
          >
            {a.label}
          </Button>
        ))}
      </div>

      {reasonFor && (
        <div className="flex flex-col gap-1.5 rounded-md border border-border-default bg-bg-subtle p-3">
          <label
            className="text-caption font-medium text-text-secondary"
            htmlFor={`suppress-reason-${findingId}`}
          >
            Why is this suppressed? (at least {MIN_SUPPRESSION_REASON_LENGTH} characters)
          </label>
          <textarea
            id={`suppress-reason-${findingId}`}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            rows={2}
            className="w-full rounded-md border border-border-default bg-bg-surface px-2.5 py-1.5 text-body-sm text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          />
          <div className="flex items-center justify-between">
            <span
              className={cn('text-caption', reasonTooShort ? 'text-warning' : 'text-text-tertiary')}
            >
              {reason.trim().length}/{MIN_SUPPRESSION_REASON_LENGTH}
            </span>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  setReasonFor(null)
                  setReason('')
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                variant="primary"
                size="sm"
                disabled={reasonTooShort}
                loading={pending === 'suppressed'}
                onClick={() => void apply('suppressed', reason)}
              >
                Confirm suppress
              </Button>
            </div>
          </div>
        </div>
      )}

      {error && (
        <p role="alert" className="text-caption text-danger">
          {error}
        </p>
      )}
    </div>
  )
}
