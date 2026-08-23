import { useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangle, ArrowRight, ChevronDown, Inbox } from 'lucide-react'
import { cn } from '../../lib/cn'
import { ENGINE_META } from '../../lib/engines'
import { humanizeErrorReason } from '../../lib/engineRunMessages'
import type { Repository } from '../../lib/projectsApi'
import type { Engine } from '../../lib/rulesApi'
import type { FindingListItem, JobStatus } from '../../lib/scansApi'
import { GitHubMark } from '../icons/GitHubMark'
import { KubernetesMark } from '../icons/KubernetesMark'
import { OsvMark } from '../icons/OsvMark'
import { SonarQubeMark } from '../icons/SonarQubeMark'
import { EmptyState } from '../ui/EmptyState'
import { FindingsList } from './FindingsList'

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

function coverageSummary(stats: Record<string, unknown> | null | undefined): string | null {
  const c = stats?.coverage
  if (!c || typeof c !== 'object') return null
  const cov = c as {
    open_ports?: number[]
    http_services_found?: number
    tls_ports_checked?: number
    total_script_runs?: number
  }
  const parts: string[] = []
  if (typeof cov.total_script_runs === 'number') parts.push(`${cov.total_script_runs} checks run`)
  if (Array.isArray(cov.open_ports)) parts.push(`${cov.open_ports.length} open ports`)
  if (typeof cov.http_services_found === 'number') {
    parts.push(
      `${cov.http_services_found} HTTP service${cov.http_services_found === 1 ? '' : 's'} probed`,
    )
  }
  if (typeof cov.tls_ports_checked === 'number' && cov.tls_ports_checked > 0) {
    parts.push(`${cov.tls_ports_checked} TLS port${cov.tls_ports_checked === 1 ? '' : 's'} checked`)
  }
  return parts.length > 0 ? parts.join(' · ') : null
}

function emptyMessage(
  engine: Engine,
  status: JobStatus,
  errorReason: string | null,
  skipReason: string | null,
  stats: Record<string, unknown> | null | undefined,
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
    case 'succeeded': {
      // pentest has no files to scan, so "came back clean" on its own reads
      // as "did nothing" rather than "checked and found nothing" — surface
      // what was actually probed instead (internal/engines/pentest/coverage.go).
      if (engine === 'pentest') {
        const summary = coverageSummary(stats)
        return summary
          ? `Target probed, no issues found — ${summary}.`
          : 'This engine came back clean — no findings.'
      }
      return 'This engine came back clean — no findings.'
    }
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
  stats = null,
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
  stats?: Record<string, unknown> | null
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
            {meta.hasSonarQubeMark && <SonarQubeMark />}
            {meta.hasKubernetesMark && <KubernetesMark />}
            {meta.hasGitHubMark && <GitHubMark className="h-3 w-3" />}
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
            status === 'skipped' ? (
              <EmptyState
                icon={Inbox}
                title={`Nothing for ${meta.label} to check`}
                description={skipReason ?? 'This engine was skipped for this scan.'}
                tone="neutral"
                size="compact"
                className="pb-4"
              />
            ) : status === 'failed' ? (
              <EmptyState
                icon={AlertTriangle}
                title="This check didn't finish"
                description={humanizeErrorReason(errorReason)}
                tone="danger"
                size="compact"
                className="pb-4"
              />
            ) : (
              <p className="pb-4 text-body-sm text-text-secondary">
                {emptyMessage(engine, status, errorReason, skipReason, stats)}
              </p>
            )
          ) : (
            <FindingsList findings={findings} repository={repository} gitRef={gitRef} />
          )}
        </div>
      )}
    </div>
  )
}
