import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { Card, CardDescription } from '../components/ui/Card'
import { EngineFindingsSection } from '../components/project/EngineFindingsSection'
import { PartialResultBanner } from '../components/project/PartialResultBanner'
import { SupplyChainPipeline } from '../components/project/SupplyChainPipeline'
import { ApiError } from '../lib/apiClient'
import { getProject, type Project } from '../lib/projectsApi'
import {
  getProgress,
  getScan,
  listFindings,
  type FindingListItem,
  type Progress,
  type Scan,
} from '../lib/scansApi'

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'cancelled'])
const POLL_INTERVAL_MS = 2000

/**
 * `/scans/:id` — the scan a "Run Scan" click lands on. Live execution graph
 * (SupplyChainPipeline) driven by `GET /scans/{id}/progress` polled every 2s
 * (FR-UI-002) while the scan is non-terminal, a PartialResultBanner once any
 * job failed/skipped, and a minimal findings list once the scan finishes.
 */
export function ScanDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [scan, setScan] = useState<Scan | null>(null)
  const [progress, setProgress] = useState<Progress | null>(null)
  const [findings, setFindings] = useState<FindingListItem[] | null>(null)
  const [project, setProject] = useState<Project | null>(null)
  const [error, setError] = useState<string | null>(null)
  const findingsLoadedFor = useRef<string | null>(null)
  const projectLoadedFor = useRef<string | null>(null)

  useEffect(() => {
    if (!id) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined

    async function tick() {
      if (!id) return
      try {
        const [scanRes, progressRes] = await Promise.all([getScan(id), getProgress(id)])
        if (cancelled) return
        setScan(scanRes)
        setProgress(progressRes)

        if (projectLoadedFor.current !== scanRes.project_id) {
          projectLoadedFor.current = scanRes.project_id
          getProject(scanRes.project_id)
            .then((res) => {
              if (!cancelled) setProject(res)
            })
            .catch(() => undefined)
        }

        if (TERMINAL_STATUSES.has(scanRes.status) && findingsLoadedFor.current !== id) {
          findingsLoadedFor.current = id
          listFindings(id)
            .then((res) => {
              if (!cancelled) setFindings(res.data)
            })
            .catch(() => undefined)
        }

        if (!TERMINAL_STATUSES.has(scanRes.status)) {
          timer = setTimeout(() => void tick(), POLL_INTERVAL_MS)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load this scan.')
        }
      }
    }

    void tick()
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
  }, [id])

  if (error) {
    return (
      <main className="mx-auto max-w-3xl px-6 py-8">
        <p role="alert" className="text-body-sm text-danger">
          {error}
        </p>
      </main>
    )
  }

  if (!scan) {
    return (
      <main className="mx-auto max-w-3xl px-6 py-8">
        <p className="text-body-sm text-text-secondary">Loading scan…</p>
      </main>
    )
  }

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <Link
        to={`/projects/${scan.project_id}/scans`}
        className="mb-4 inline-flex items-center gap-1.5 text-body-sm text-text-secondary hover:text-text-primary"
      >
        <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        Back to scans
      </Link>

      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-h1 text-text-primary">Scan {scan.id.slice(0, 8)}</h1>
          <p className="text-body-sm text-text-secondary">
            {scan.type.replace(/_/g, ' ')} · queued {new Date(scan.queued_at).toLocaleString()}
          </p>
        </div>
        <span className="rounded-full bg-bg-subtle px-3 py-1 text-body-sm font-medium capitalize text-text-primary">
          {scan.status}
        </span>
      </div>

      <PartialResultBanner jobs={scan.jobs} />

      <div className="mb-6">
        <SupplyChainPipeline progress={progress} jobs={scan.jobs} scan={scan} />
      </div>

      <div>
        <h2 className="mb-3 text-h3 text-text-primary">Findings</h2>
        {!TERMINAL_STATUSES.has(scan.status) && (
          <Card>
            <CardDescription>Findings will appear once the scan finishes.</CardDescription>
          </Card>
        )}
        {TERMINAL_STATUSES.has(scan.status) && findings === null && (
          <Card>
            <CardDescription>Loading findings…</CardDescription>
          </Card>
        )}
        {findings !== null && (
          // One section per engine that actually ran, not a flat mixed
          // list — the same grouping ProjectFindingsPanel already uses, so
          // it's immediately clear which engine (Kubernetes, Code, Deps,
          // Containers, ...) a given finding came from, rather than having
          // to read every rule_id to tell them apart.
          <div className="flex flex-col gap-3">
            {scan.jobs.map((job) => (
              <EngineFindingsSection
                key={job.id}
                engine={job.engine}
                status={job.status}
                findingCount={job.finding_count}
                findings={findings.filter((f) => f.engine === job.engine)}
                scanId={scan.id}
                repository={project?.repository ?? null}
                gitRef={
                  scan.commit_sha ?? scan.branch ?? project?.repository?.default_branch ?? null
                }
                errorReason={job.error_reason}
                skipReason={job.skip_reason}
                defaultExpanded={scan.jobs.length === 1}
              />
            ))}
          </div>
        )}
      </div>
    </main>
  )
}
