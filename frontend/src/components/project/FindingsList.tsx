import { groupFindingsByRule } from '../../lib/groupFindings'
import type { Repository } from '../../lib/projectsApi'
import type { FindingListItem } from '../../lib/scansApi'
import { FindingGroupRow } from './FindingGroupRow'

/**
 * The one place a flat findings array becomes a rendered list — always
 * called with one engine's findings already filtered out by
 * EngineFindingsSection (the per-project/global Findings tabs, and
 * ScanDetailPage's per-engine sections), never a mixed-engine list of its
 * own, so grouping behaves identically everywhere rather than being
 * reimplemented (and risking drifting out of sync) per page.
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
