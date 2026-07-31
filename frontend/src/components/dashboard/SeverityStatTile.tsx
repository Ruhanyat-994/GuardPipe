export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info'

const LABEL: Record<Severity, string> = {
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
  info: 'Info',
}

/**
 * The one deliberate "loud colour" exception in the whole product
 * (documentation/09-ui-ux-design-system.md §1 and §4.2, `SeverityStatTile`)
 * — modelled directly on the Tenable/SecurityCenter reference screenshots:
 * solid severity-colour blocks, scannable from across the room. Everything
 * else in the app uses colour as a restrained accent.
 */
export function SeverityStatTile({
  severity,
  count,
  delta,
}: {
  severity: Severity
  count: number
  /** Change since the previous scan — negative reads as good news (fewer). */
  delta?: number
}) {
  return (
    <div
      className="relative overflow-hidden rounded-lg p-4 text-white"
      style={{ backgroundColor: `var(--sev-${severity})` }}
    >
      {delta !== undefined && (
        <span className="absolute top-3 right-3 rounded-full bg-black/20 px-2 py-0.5 text-caption font-semibold">
          {delta > 0 ? `+${delta}` : delta}
        </span>
      )}
      <div className="text-display font-semibold">{count}</div>
      <div className="mt-1 text-caption font-semibold tracking-wide text-white/85 uppercase">
        {LABEL[severity]}
      </div>
    </div>
  )
}
