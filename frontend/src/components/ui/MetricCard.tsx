import { Info } from 'lucide-react'
import { Card } from './Card'
import { cn } from '../../lib/cn'

/**
 * Neutral stat card (documentation/09-ui-ux-design-system.md §4.4) —
 * number + label + optional tooltip, no colour coding. Distinct from
 * `SeverityStatTile`, which stays the one deliberate loud-colour exception
 * for severity counts specifically (principle 7). `MetricCard` is for
 * everything else: "Repositories connected," "Targets awaiting
 * attestation," and similar counts that don't carry a severity meaning.
 */
export function MetricCard({
  label,
  value,
  caption,
  tooltip,
  className,
}: {
  label: string
  value: string | number
  caption?: string
  tooltip?: string
  className?: string
}) {
  return (
    <Card className={cn('p-4', className)}>
      <div className="flex items-center gap-1 text-body-sm text-text-secondary">
        {label}
        {tooltip && (
          <span title={tooltip}>
            <Info className="h-3.5 w-3.5 text-text-tertiary" aria-hidden="true" />
          </span>
        )}
      </div>
      <div className="mt-1 text-h1 text-text-primary">{value}</div>
      {caption && <div className="mt-0.5 text-caption text-text-tertiary">{caption}</div>}
    </Card>
  )
}
