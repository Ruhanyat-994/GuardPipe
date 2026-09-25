import { useEffect, useRef, useState } from 'react'
import { Coins } from 'lucide-react'
import { cn } from '../../lib/cn'
import { ENGINE_META } from '../../lib/engines'
import { describeReason, formatTokens, getScanTokens, type ScanTokens } from '../../lib/billingApi'
import { notifyBillingChanged } from '../../lib/billingEvents'

/**
 * "20,500 tokens · 4,000 refunded" for one scan. Re-reads whenever a job's
 * status changes (statusKey), and when a new refund lands it nudges the
 * top-bar token bar so the refund animates in.
 */
export function ScanTokensChip({
  scanId,
  statusKey,
  className,
}: {
  scanId: string
  statusKey: string
  className?: string
}) {
  const [tokens, setTokens] = useState<ScanTokens | null>(null)
  const lastRefunded = useRef<number | null>(null)

  useEffect(() => {
    let cancelled = false
    getScanTokens(scanId)
      .then((t) => {
        if (cancelled) return
        setTokens(t)
        if (lastRefunded.current !== null && t.refunded > lastRefunded.current)
          notifyBillingChanged()
        lastRefunded.current = t.refunded
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [scanId, statusKey])

  if (!tokens || tokens.charged === 0) return null
  const refunds = tokens.entries.filter((e) => e.kind === 'refund')
  const title = refunds.length
    ? refunds
        .map(
          (r) =>
            `+${formatTokens(r.delta)} ${r.engine ? (ENGINE_META[r.engine]?.label ?? r.engine) : ''}: ${describeReason(r.reason)}`,
        )
        .join('\n')
    : 'Token cost of this scan'

  return (
    <span
      title={title}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border border-border-default bg-bg-surface px-2.5 py-0.5 text-caption text-text-secondary',
        className,
      )}
    >
      <Coins className="h-3.5 w-3.5 text-accent" aria-hidden="true" />
      <span className="tabular-nums text-text-primary">
        {formatTokens(tokens.charged - tokens.refunded)}
      </span>{' '}
      tokens
      {tokens.refunded > 0 && (
        <span className="tabular-nums text-success">
          · {formatTokens(tokens.refunded)} refunded
        </span>
      )}
    </span>
  )
}
