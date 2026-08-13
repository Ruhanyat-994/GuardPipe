import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, FolderKanban, ShieldQuestion } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { ProjectFindingsPanel } from '../components/project/ProjectFindingsPanel'
import { ApiError } from '../lib/apiClient'
import { listProjects, type Project } from '../lib/projectsApi'
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

/**
 * The global Findings page — every project, each collapsible down to its
 * most recent scan's findings grouped by engine (ProjectFindingsPanel,
 * shared with the per-project Findings tab). "Project name -> scan type ->
 * findings" as one browsable hierarchy instead of having to open each
 * project individually.
 */
export function GlobalFindingsPage() {
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
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
  }, [])

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Findings</h1>
        <p className="text-body-sm text-text-secondary">
          Every project's most recent findings, grouped by engine. Open a project to drill in.
        </p>
      </div>

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
    </main>
  )
}
