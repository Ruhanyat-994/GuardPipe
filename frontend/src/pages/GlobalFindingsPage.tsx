import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { ChevronDown, FolderKanban, ShieldQuestion } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { FindingsList } from '../components/project/FindingsList'
import { ProjectFindingsPanel } from '../components/project/ProjectFindingsPanel'
import { ApiError } from '../lib/apiClient'
import { listProjects, type Project } from '../lib/projectsApi'
import { listFindings, listOrgScans, type FindingListItem, type OrgScanSummary } from '../lib/scansApi'
import { cn } from '../lib/cn'

/** One project's row — collapsed by default so opening this page doesn't
 * fire a findings fetch per project; ProjectFindingsPanel only mounts (and
 * only then fetches) once its project is expanded. */
function ProjectFindingsAccordion({ project }: { project: Project }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="overflow-hidden rounded-lg border border-border-default bg-bg-surface">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center gap-3 px-5 py-4 text-left"
        aria-expanded={expanded}
      >
        <FolderKanban className="h-4 w-4 shrink-0 text-text-secondary" aria-hidden="true" />
        <span className="flex-1 font-semibold text-text-primary">{project.name}</span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 text-text-tertiary transition-transform',
            expanded && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </button>
      {expanded && (
        <div className="border-t border-border-default px-5 py-4">
          <ProjectFindingsPanel projectId={project.id} repository={project.repository} />
        </div>
      )}
    </div>
  )
}

interface SeverityRow {
  project: Project
  findings: FindingListItem[]
  gitRef: string | null
}

/**
 * The flat, cross-project view a dashboard severity-tile click lands on
 * (`?severity=critical` etc.) — every project's *most recent completed*
 * scan (same "current picture" the dashboard's own totals are built from,
 * see GlobalDashboardPage's totals comment), filtered down to just that
 * severity, grouped by project with a project-name link straight to
 * `/projects/:id`. Deliberately uncapped (every project, not the
 * dashboard's top-3) — this is one explicit user click, not a page-load
 * fetch, and completeness is the entire point of clicking a severity total.
 */
function SeverityFindingsView({ severity }: { severity: string }) {
  const [rows, setRows] = useState<SeverityRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    Promise.all([listProjects(), listOrgScans(1, 100)])
      .then(async ([projectsRes, scansRes]) => {
        if (cancelled) return

        const latestByProject = new Map<string, OrgScanSummary>()
        for (const s of scansRes.data) {
          if (!latestByProject.has(s.project_id)) latestByProject.set(s.project_id, s)
        }
        const candidates = projectsRes.data
          .map((project) => ({ project, scan: latestByProject.get(project.id) ?? null }))
          .filter(
            (r): r is { project: Project; scan: OrgScanSummary } => r.scan?.status === 'completed',
          )

        const results = await Promise.all(
          candidates.map(({ project, scan }) =>
            listFindings(scan.id)
              .then(
                (res): SeverityRow => ({
                  project,
                  findings: res.data.filter((f) => f.severity === severity),
                  gitRef: scan.branch ?? project.repository?.default_branch ?? null,
                }),
              )
              .catch((): SeverityRow => ({ project, findings: [], gitRef: null })),
          ),
        )
        if (cancelled) return
        setRows(results.filter((r) => r.findings.length > 0))
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load findings.')
      })

    return () => {
      cancelled = true
    }
  }, [severity])

  if (error) {
    return (
      <Card className="border-danger/30 bg-danger/5">
        <p role="alert" className="text-body-sm text-danger">
          {error}
        </p>
      </Card>
    )
  }

  if (rows === null) {
    return <p className="text-body-sm text-text-secondary">Loading findings…</p>
  }

  if (rows.length === 0) {
    return (
      <Card className="flex flex-col items-center gap-3 py-16 text-center">
        <ShieldQuestion className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
        <CardTitle className="capitalize">No {severity} findings</CardTitle>
        <CardDescription className="max-w-sm">
          None of your projects' most recent scans have a finding at this severity right now.
        </CardDescription>
      </Card>
    )
  }

  const total = rows.reduce((sum, r) => sum + r.findings.length, 0)

  return (
    <div className="flex flex-col gap-3">
      <p className="text-body-sm text-text-secondary">
        {total} {severity} finding{total === 1 ? '' : 's'} across {rows.length} project
        {rows.length === 1 ? '' : 's'}.
      </p>
      {rows.map(({ project, findings, gitRef }) => (
        <Card key={project.id} className="p-0">
          <Link
            to={`/projects/${project.id}`}
            className="flex items-center gap-3 border-b border-border-default px-5 py-4 hover:bg-bg-subtle"
          >
            <FolderKanban className="h-4 w-4 shrink-0 text-text-secondary" aria-hidden="true" />
            <span className="flex-1 font-semibold text-text-primary hover:underline">
              {project.name}
            </span>
            <span className="shrink-0 rounded-full bg-bg-subtle px-2 py-0.5 text-caption font-semibold text-text-secondary">
              {findings.length}
            </span>
          </Link>
          <div className="px-5">
            <FindingsList findings={findings} repository={project.repository} gitRef={gitRef} />
          </div>
        </Card>
      ))}
    </div>
  )
}

/**
 * The global Findings page — every project, each collapsible down to its
 * most recent scan's findings grouped by engine (ProjectFindingsPanel,
 * shared with the per-project Findings tab). "Project name -> scan type ->
 * findings" as one browsable hierarchy instead of having to open each
 * project individually. With a `?severity=` param (a dashboard severity-tile
 * click), this switches to the flat cross-project SeverityFindingsView above
 * instead.
 */
export function GlobalFindingsPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const severity = searchParams.get('severity')
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (severity) return // the severity view fetches its own data
    let cancelled = false
    listProjects()
      .then((res) => {
        if (!cancelled) setProjects(res.data)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load projects.')
      })
    return () => {
      cancelled = true
    }
  }, [severity])

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Findings</h1>
        <p className="text-body-sm text-text-secondary">
          {severity
            ? "Every project's findings at this severity, from each project's most recent scan."
            : "Every project's most recent findings, grouped by engine. Open a project to drill in."}
        </p>
      </div>

      {severity && (
        <p className="mb-4 text-body-sm text-text-secondary">
          Filtered to <span className="font-medium capitalize text-text-primary">{severity}</span>{' '}
          ·{' '}
          <Link
            to="?"
            replace
            className="text-accent hover:underline"
            onClick={(e) => {
              e.preventDefault()
              setSearchParams({})
            }}
          >
            Clear filter
          </Link>
        </p>
      )}

      {severity && <SeverityFindingsView key={severity} severity={severity} />}

      {!severity && (
        <>
          {error && (
            <Card className="border-danger/30 bg-danger/5">
              <p role="alert" className="text-body-sm text-danger">
                {error}
              </p>
            </Card>
          )}

          {!error && projects === null && (
            <p className="text-body-sm text-text-secondary">Loading projects…</p>
          )}

          {!error && projects !== null && projects.length === 0 && (
            <Card className="flex flex-col items-center gap-3 py-16 text-center">
              <ShieldQuestion className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
              <CardTitle>No projects yet</CardTitle>
              <CardDescription className="max-w-sm">
                Create a project and run a scan to see findings here.
              </CardDescription>
              <Button onClick={() => navigate('/projects/new')} className="mt-2">
                New Project
              </Button>
            </Card>
          )}

          {!error && projects !== null && projects.length > 0 && (
            <div className="flex flex-col gap-3">
              {projects.map((p) => (
                <ProjectFindingsAccordion key={p.id} project={p} />
              ))}
            </div>
          )}
        </>
      )}
    </main>
  )
}
