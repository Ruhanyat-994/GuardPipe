import type { FindingListItem } from './scansApi'
import type { Severity } from './rulesApi'

/**
 * One rule's worth of findings from a single scan. The same CVE/rule
 * routinely fires on several genuinely distinct locations in one scan — a
 * vulnerable OS library pulled in by several packages (containerscan), the
 * same dependency declared in several manifests (depscan), the same
 * SonarQube rule on several files (codescan). Each of those is a real,
 * independently-remediable finding — none of them should be discarded —
 * but showing all of them as flat, near-identical rows reads as broken or
 * spammy output rather than as what it actually is. Grouping is purely a
 * display concern: `items` still holds every finding GuardPipe detected.
 */
export interface FindingGroup {
  ruleId: string
  severity: Severity
  representative: FindingListItem
  items: FindingListItem[]
}

/**
 * Groups findings by `rule_id`, preserving each rule's first-seen order.
 * Severity for the group is the worst (numerically lowest-ranked) severity
 * among its members, so a group never under-represents its own risk.
 */
export function groupFindingsByRule(findings: FindingListItem[]): FindingGroup[] {
  const order: string[] = []
  const byRule = new Map<string, FindingListItem[]>()

  for (const f of findings) {
    const existing = byRule.get(f.rule_id)
    if (existing) {
      existing.push(f)
    } else {
      byRule.set(f.rule_id, [f])
      order.push(f.rule_id)
    }
  }

  return order.map((ruleId) => {
    const items = byRule.get(ruleId)!
    return {
      ruleId,
      severity: worstSeverity(items),
      representative: items[0],
      items,
    }
  })
}

const SEVERITY_RANK: Record<Severity, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  informational: 4,
}

function worstSeverity(items: FindingListItem[]): Severity {
  return items.reduce(
    (worst, f) => (SEVERITY_RANK[f.severity] < SEVERITY_RANK[worst] ? f.severity : worst),
    items[0].severity,
  )
}
