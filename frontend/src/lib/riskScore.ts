import type { Verdict } from '../components/dashboard/RiskGauge'

/**
 * A provisional, severity-weighted score — not GuardPipe's real risk
 * scoring (that's Phase 13, not built yet). Deliberately simple and stated
 * as such everywhere it's shown: CLAUDE.md's standing rule is that a risk
 * score must be defensible, not implied to be more authoritative than it
 * is. This exists so the dashboard has a real, honestly-labelled hero
 * number today instead of either a fabricated "real" score or no number at
 * all — every input is a real finding count from a real scan, only the
 * weighting formula itself is a placeholder.
 */
export function computeRiskScore(counts: Record<string, number>): {
  score: number
  verdict: Verdict
} {
  const critical = counts.critical ?? 0
  const high = counts.high ?? 0
  const medium = counts.medium ?? 0
  const low = counts.low ?? 0

  const raw = critical * 20 + high * 8 + medium * 3 + low * 1
  const score = Math.min(100, raw)

  const verdict: Verdict = score >= 70 ? 'block' : score >= 35 ? 'warn' : 'pass'

  return { score, verdict }
}

/** Sums a set of per-severity count maps into one totals map — the shared
 * aggregation step both the global (across projects) and project (across
 * jobs) dashboards need. */
export function sumCounts(all: Record<string, number>[]): Record<string, number> {
  const totals: Record<string, number> = {}
  for (const counts of all) {
    for (const [severity, count] of Object.entries(counts)) {
      totals[severity] = (totals[severity] ?? 0) + count
    }
  }
  return totals
}
