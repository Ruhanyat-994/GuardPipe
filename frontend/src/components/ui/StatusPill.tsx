import { AlertCircle, Check, CircleDashed, EyeOff, RotateCcw } from 'lucide-react'
import { cn } from '../../lib/cn'
import type { FindingStatus } from '../../lib/scansApi'

/**
 * `StatusPill` (documentation/09-ui-ux-design-system.md §4.2): open ·
 * acknowledged · suppressed · fixed · false_positive — neutral colours,
 * since status is not severity (a suppressed critical is still critical,
 * just triaged). `fixed` gets a light success tint as the one deliberate
 * exception — it's a genuinely good outcome, not a neutral one, and
 * documentation/11-risk-scoring-and-severity.md §5 already treats it as
 * meaningfully different (excluded from the score, not just dimmed).
 */
const CONFIG: Record<FindingStatus, { label: string; icon: typeof Check; className: string }> = {
  open: {
    label: 'Open',
    icon: CircleDashed,
    className: 'border-border-default bg-bg-subtle text-text-secondary',
  },
  acknowledged: {
    label: 'Acknowledged',
    icon: AlertCircle,
    className: 'border-border-default bg-bg-subtle text-text-secondary',
  },
  suppressed: {
    label: 'Suppressed',
    icon: EyeOff,
    className: 'border-border-default bg-bg-subtle text-text-tertiary',
  },
  false_positive: {
    label: 'False positive',
    icon: RotateCcw,
    className: 'border-border-default bg-bg-subtle text-text-tertiary',
  },
  fixed: {
    label: 'Fixed',
    icon: Check,
    className: 'border-success/30 bg-success/10 text-success',
  },
}

export function StatusPill({ status, className }: { status: FindingStatus; className?: string }) {
  const cfg = CONFIG[status]
  const Icon = cfg.icon
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-caption font-semibold',
        cfg.className,
        className,
      )}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {cfg.label}
    </span>
  )
}
