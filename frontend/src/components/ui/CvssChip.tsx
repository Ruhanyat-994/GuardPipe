import { cn } from '../../lib/cn'

const SEVERITY_COLOR: Record<string, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  informational: 'var(--sev-info)',
}

/** CVSS v3.1 band thresholds (documentation/11-risk-scoring-and-severity.md
 * §2.2) — this chip is purely presentational, so it re-derives the colour
 * band from the score itself rather than trusting a caller-supplied
 * severity that might belong to a context modifier, not the raw CVSS
 * value this chip specifically represents. */
function bandColor(score: number): string {
  if (score >= 9.0) return SEVERITY_COLOR.critical
  if (score >= 7.0) return SEVERITY_COLOR.high
  if (score >= 4.0) return SEVERITY_COLOR.medium
  if (score > 0) return SEVERITY_COLOR.low
  return SEVERITY_COLOR.informational
}

/**
 * `CvssChip` (documentation/09-ui-ux-design-system.md §4.2): score +
 * severity colour, vector on hover. Renders nothing for a finding with no
 * CVSS score (most rule-based findings — CVSS applies to CVE-backed
 * dependency/image vulnerabilities, not every finding type).
 */
export function CvssChip({
  score,
  vector,
  className,
}: {
  score: number | null | undefined
  vector?: string | null
  className?: string
}) {
  if (score == null) return null
  const color = bandColor(score)

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-caption font-semibold',
        className,
      )}
      style={{
        color,
        borderColor: color,
        backgroundColor: `color-mix(in srgb, ${color} 10%, transparent)`,
      }}
      title={vector ?? undefined}
    >
      CVSS {score.toFixed(1)}
    </span>
  )
}
