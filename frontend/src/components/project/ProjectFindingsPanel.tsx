import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, ScanSearch } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { EngineFindingsSection } from './EngineFindingsSection'
import { ApiError } from '../../lib/apiClient'
import type { Repository } from '../../lib/projectsApi'
import {
  getScan,
  listFindings,
  listScans,
  type FindingListItem,
  type Scan,
} from '../../lib/scansApi'
import { relativeTime } from '../../lib/format'

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'cancelled'])

/**
 * Findings from one project's most recent scan, grouped by engine — the
 * shared body behind both the per-project Findings tab
 * (`ProjectFindingsPage`, one of these already expanded) and the global
 * Findings page (one collapsed per project, this mounted only once
 * expanded so visiting the page doesn't fire a fetch per project up
 * front). Grouping is by `scan.jobs`, the true set of what ran — never a
 * guessed/fabricated engine list.
 */
export function ProjectFindingsPanel({
  projectId,
  repository,
  severityFilter,
}: {
  projectId: string
  repository: Repository | null
  /** Backend severity string (e.g. "critical", "informational") — when set,
   * only findings of this severity are shown, in every engine section. */
  severityFilter?: string
}) {
  const [scan, setScan] = useState<Scan | null | undefined>(undefined) // undefined = loading, null = no scans yet
  const [findings, setFindings] = useState<FindingListItem[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    listScans(projectId, 1, 1)
      .then((res) => {
        if (cancelled) return
        if (res.data.length === 0) {
          setScan(null)
          return
        }
        return getScan(res.data[0].id).then((full) => {
          if (cancelled) return
          setScan(full)
          if (TERMINAL_STATUSES.has(full.status)) {
            return listFindings(full.id).then((findingsRes) => {
              if (!cancelled) setFindings(findingsRes.data)
            })
          }
        })
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load findings.')
      })

    return () => {
      cancelled = true
    }
  }, [projectId])

  if (error) {
    return (
      <Card className="border-danger/30 bg-danger/5">
        <p role="alert" className="text-body-sm text-danger">
          {error}
        </p>
      </Card>
    )
  }

  if (scan === undefined) {
    return <p className="text-body-sm text-text-secondary">Loading findings…</p>
  }

  if (scan === null) {
    return (
      <Card className="flex flex-col items-center gap-3 py-16 text-center">
        <ScanSearch className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
        <CardTitle>No scans yet</CardTitle>
        <CardDescription className="max-w-sm">
          Run a scan from the Scans tab to see dependency, secret, and vulnerability findings here.
        </CardDescription>
        <Link
          to={`/projects/${projectId}/scans`}
          className="mt-1 inline-flex items-center gap-1 text-body-sm font-medium text-accent hover:underline"
        >
          Go to Scans
          <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
        </Link>
      </Card>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2 px-1">
        <p className="text-body-sm text-text-secondary">
          From scan <span className="font-mono text-text-primary">{scan.id.slice(0, 8)}</span> ·
          queued {relativeTime(scan.queued_at)}
        </p>
        <Link
          to={`/scans/${scan.id}`}
          className="inline-flex items-center gap-1 text-body-sm font-medium text-accent hover:underline"
        >
          View scan
          <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
        </Link>
      </div>

      {!TERMINAL_STATUSES.has(scan.status) && (
        <Card>
          <CardDescription>
            This scan is still running — findings will appear here once it finishes. Open the scan
            to watch its progress live.
          </CardDescription>
        </Card>
      )}

      {TERMINAL_STATUSES.has(scan.status) && findings === null && (
        <Card>
          <CardDescription>Loading findings…</CardDescription>
        </Card>
      )}

      {findings !== null &&
        (() => {
          const visible = severityFilter
            ? findings.filter((f) => f.severity === severityFilter)
            : findings
          // With a severity filter active, an engine that ran clean for
          // that severity has nothing useful to show — skip its section
          // entirely rather than a wall of "no findings" cards.
          const jobs = severityFilter
            ? scan.jobs.filter((job) => visible.some((f) => f.engine === job.engine))
            : scan.jobs
          if (severityFilter && jobs.length === 0) {
            return (
              <Card>
                <CardDescription>
                  No {severityFilter} findings in this project's most recent scan.
                </CardDescription>
              </Card>
            )
          }
          return jobs.map((job) => (
            <EngineFindingsSection
              key={job.id}
              engine={job.engine}
              status={job.status}
              findingCount={
                severityFilter
                  ? visible.filter((f) => f.engine === job.engine).length
                  : job.finding_count
              }
              findings={visible.filter((f) => f.engine === job.engine)}
              scanId={scan.id}
              repository={repository}
              gitRef={scan.commit_sha ?? scan.branch ?? repository?.default_branch ?? null}
              errorReason={job.error_reason}
              skipReason={job.skip_reason}
              stats={job.stats}
              defaultExpanded={scan.jobs.length === 1 || Boolean(severityFilter)}
            />
          ))
        })()}
    </div>
  )
}
