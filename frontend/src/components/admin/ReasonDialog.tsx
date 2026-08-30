import { useState } from 'react'
import { Button } from '../ui/Button'
import { Card } from '../ui/Card'

/**
 * A reason-required confirmation modal — never a bare confirm() dialog
 * (BUILD_GUIDE.md Phase 14) — used for every suspend action across the
 * admin panel. The reason is what lands in audit_log.detail, so the UI has
 * to make skipping it impossible, not just discouraged: the confirm button
 * stays disabled until at least one non-whitespace character is entered.
 */
export function ReasonDialog({
  title,
  description,
  confirmLabel = 'Confirm',
  onConfirm,
  onCancel,
}: {
  title: string
  description?: string
  confirmLabel?: string
  onConfirm: (reason: string) => void | Promise<void>
  onCancel: () => void
}) {
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState(false)
  const trimmed = reason.trim()

  async function handleConfirm() {
    if (!trimmed) return
    setPending(true)
    try {
      await onConfirm(trimmed)
    } finally {
      setPending(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onClick={onCancel}
    >
      <Card
        className="w-full max-w-md"
        role="dialog"
        aria-modal="true"
        aria-labelledby="reason-dialog-title"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="reason-dialog-title" className="text-h3 text-text-primary">
          {title}
        </h2>
        {description && <p className="mt-1 text-body-sm text-text-secondary">{description}</p>}

        <label className="mt-4 block text-body-sm font-medium text-text-primary">
          Reason
          <textarea
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            rows={3}
            className="mt-1.5 w-full rounded-md border border-border-default bg-bg-surface px-3 py-2 text-body text-text-primary placeholder:text-text-tertiary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2"
            placeholder="Why is this action being taken? This is recorded in the audit log."
          />
        </label>

        <div className="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancel} disabled={pending}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={!trimmed}
            loading={pending}
            onClick={() => void handleConfirm()}
          >
            {confirmLabel}
          </Button>
        </div>
      </Card>
    </div>
  )
}
