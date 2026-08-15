import { groupFindingsByRule } from '../../lib/groupFindings'
import type { Repository } from '../../lib/projectsApi'
import type { FindingListItem } from '../../lib/scansApi'
import { FindingGroupRow } from './FindingGroupRow'

/**
 * The one place a flat findings array becomes a rendered list — every
 * surface that lists findings (the per-project/global Findings tabs via
 * EngineFindingsSection, and ScanDetailPage's own inline list) renders
 * through this, so grouping behaves identically everywhere rather than
 * being reimplemented (and risking drifting out of sync) per page.
 */
export function FindingsList({
  findings,
  repository,
  gitRef,
}: {
  findings: FindingListItem[]
  repository: Repository | null
  gitRef: string | null
}) {
  const groups = groupFindingsByRule(findings)
  return (
    <ul className="flex flex-col divide-y divide-border-default">
      {groups.map((group) => (
        <FindingGroupRow key={group.ruleId} group={group} repository={repository} gitRef={gitRef} />
      ))}
    </ul>
  )
}
