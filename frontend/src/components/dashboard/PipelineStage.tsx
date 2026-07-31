import type { LucideIcon } from 'lucide-react'

export type StageStatus = 'critical' | 'high' | 'medium' | 'low' | 'failed' | 'skipped' | 'not_run'

const STATUS_COLOR: Record<StageStatus, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  failed: 'var(--danger)',
  skipped: 'var(--text-tertiary)',
  not_run: 'var(--text-tertiary)',
}

/**
 * One stage of the `SupplyChainPipeline` — the signature visual
 * (documentation/09-ui-ux-design-system.md §4.2, FR-UI-004). Colour is the
 * worst severity found by that engine, or a neutral tone for
 * skipped/not_run — a skip is not a failure (documentation/03-architecture-overview.md
 * §6.3).
 */
export function PipelineStage({
  label,
  detail,
  status,
  icon: Icon,
}: {
  label: string
  detail: string
  status: StageStatus
  icon: LucideIcon
}) {
  const color = STATUS_COLOR[status]
  return (
    <div className="flex min-w-[92px] flex-col items-center gap-2 rounded-lg border border-border-default bg-bg-surface px-3 py-4 text-center shadow-sm">
      <div
        className="flex h-10 w-10 items-center justify-center rounded-full"
        style={{ backgroundColor: `color-mix(in srgb, ${color} 15%, transparent)`, color }}
      >
        <Icon className="h-5 w-5" aria-hidden="true" />
      </div>
      <span className="text-body-sm font-medium text-text-primary">{label}</span>
      <span className="text-caption" style={{ color }}>
        {detail}
      </span>
    </div>
  )
}
