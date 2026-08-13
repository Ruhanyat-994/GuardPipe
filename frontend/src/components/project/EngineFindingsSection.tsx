import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, ChevronDown } from 'lucide-react'
import { cn } from '../../lib/cn'
import { ENGINE_META } from '../../lib/engines'
import type { Repository } from '../../lib/projectsApi'
import type { Engine } from '../../lib/rulesApi'
import type { FindingListItem, JobStatus } from '../../lib/scansApi'
import { OsvMark } from '../icons/OsvMark'
import { FindingRow } from './FindingRow'

const STATUS_COLOR: Record<JobStatus, string> = {
  queued: 'var(--text-tertiary)',
  running: 'var(--accent)',
  succeeded: 'var(--success)',
  failed: 'var(--danger)',
  skipped: 'var(--text-tertiary)',
  cancelled: 'var(--text-tertiary)',
}

function subtitle(status: JobStatus, findingCount: number): string {
  switch (status) {
    case 'queued':
      return 'Queued'
    case 'running':
      return 'Running…'
    case 'failed':
      return 'Failed'
    case 'skipped':
      return 'Skipped'
    case 'cancelled':
      return 'Cancelled'
    case 'succeeded':
      return findingCount === 0
        ? 'No findings — clean'
        : `${findingCount} finding${findingCount === 1 ? '' : 's'}`
  }
}

function emptyMessage(
  status: JobStatus,
  errorReason: string | null,
  skipReason: string | null,
): string {
  switch (status) {
    case 'queued':
    case 'running':
      return 'Still in progress — open the scan to watch it live.'
    case 'failed':
      return errorReason ?? 'This engine failed to complete.'
    case 'skipped':
      return skipReason ?? 'This engine was skipped for this scan.'
    case 'cancelled':
      return 'This scan was cancelled before this engine finished.'
    case 'succeeded':
      return 'This engine came back clean — no findings.'
  }
}

/**
 * One engine's findings from the project's most recent scan, grouped under
 * that engine (e.g. "Deps") rather than a flat undifferentiated list — one
 * card per job the scan actually ran (documentation/03-architecture-overview.md's
 * per-engine Engine interface means a scan's jobs already are the true set
 * of "what ran," so this never has to guess or fake which engines
 * contributed). Collapsed by default; expanding both reveals its findings
 * (each further expandable via FindingRow's own "More") and links to the
 * full scan page.
 */
export function EngineFindingsSection({
  engine,
  status,
  findingCount,
  findings,
  scanId,
  repository,
  gitRef,
  errorReason,
  skipReason,
  defaultExpanded = false,
}: {
  engine: Engine
  status: JobStatus
  findingCount: number
  findings: FindingListItem[]
  scanId: string
  repository: Repository | null
  gitRef: string | null
  errorReason: string | null
  skipReason: string | null
  defaultExpanded?: boolean
}) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const meta = ENGINE_META[engine]
  const Icon = meta.icon
  const color = STATUS_COLOR[status]

  return (
    <div className="overflow-hidden rounded-lg border border-border-default bg-bg-surface">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center gap-3 px-5 py-4 text-left"
        aria-expanded={expanded}
      >
        <div
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full"
          style={{ backgroundColor: `color-mix(in srgb, ${color} 15%, transparent)`, color }}
        >
          <Icon className="h-4.5 w-4.5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="font-semibold text-text-primary">{meta.label}</span>
            {meta.hasOsvMark && <OsvMark />}
          </div>
          <span className="text-caption" style={{ color }}>
            {subtitle(status, findingCount)}
          </span>
        </div>
        {findingCount > 0 && (
          <span className="shrink-0 rounded-full bg-bg-subtle px-2 py-0.5 text-caption font-semibold text-text-secondary">
            {findingCount}
          </span>
        )}
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 text-text-tertiary transition-transform',
            expanded && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </button>

      {expanded && (
        <div className="border-t border-border-default px-5 pb-2">
          <div className="flex justify-end py-2.5">
            <Link
              to={`/scans/${scanId}`}
              className="inline-flex items-center gap-1 text-caption font-medium text-accent hover:underline"
            >
              View scan
              <ArrowRight className="h-3 w-3" aria-hidden="true" />
            </Link>
          </div>
          {findings.length === 0 ? (
            <p className="pb-4 text-body-sm text-text-secondary">
              {emptyMessage(status, errorReason, skipReason)}
            </p>
          ) : (
            <ul className="flex flex-col divide-y divide-border-default">
              {findings.map((f) => (
                <FindingRow key={f.id} finding={f} repository={repository} gitRef={gitRef} />
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}
