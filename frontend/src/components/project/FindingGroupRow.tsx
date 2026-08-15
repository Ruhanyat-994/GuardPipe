import { useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '../../lib/cn'
import type { FindingGroup } from '../../lib/groupFindings'
import type { Repository } from '../../lib/projectsApi'
import { FindingRow } from './FindingRow'

const SEVERITY_COLOR: Record<string, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  informational: 'var(--sev-info)',
}

/**
 * One rule's findings. A single occurrence renders exactly as a bare
 * `FindingRow` always has — no visual change for the common case. Multiple
 * occurrences (the same CVE/rule hitting several distinct packages, files,
 * or manifests) collapse under one header carrying the count, instead of
 * repeating a near-identical-looking row once per occurrence — every one of
 * those findings is still real and still here, just nested one level, so
 * nothing GuardPipe detected is ever hidden by this.
 */
export function FindingGroupRow({
  group,
  repository,
  gitRef,
}: {
  group: FindingGroup
  repository: Repository | null
  gitRef: string | null
}) {
  const [expanded, setExpanded] = useState(false)

  if (group.items.length === 1) {
    return <FindingRow finding={group.items[0]} repository={repository} gitRef={gitRef} />
  }

  return (
    <li className="py-3">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center gap-3 text-left"
        aria-expanded={expanded}
      >
        <span
          className="shrink-0 rounded-full px-2 py-0.5 text-caption font-semibold text-white uppercase"
          style={{ backgroundColor: SEVERITY_COLOR[group.severity] }}
        >
          {group.severity}
        </span>
        <span className="flex-1 text-body-sm text-text-primary">{group.representative.title}</span>
        <span className="shrink-0 rounded-full bg-bg-subtle px-2 py-0.5 text-caption font-semibold text-text-secondary">
          {group.items.length} occurrences
        </span>
        <span className="hidden text-caption text-text-tertiary sm:inline">{group.ruleId}</span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 text-text-tertiary transition-transform',
            expanded && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </button>

      {expanded && (
        <ul className="mt-2 ml-1 flex flex-col divide-y divide-border-default border-l-2 border-border-default pl-4">
          {group.items.map((f) => (
            <FindingRow key={f.id} finding={f} repository={repository} gitRef={gitRef} />
          ))}
        </ul>
      )}
    </li>
  )
}
