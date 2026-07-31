import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FolderGit2, FolderPlus, Plus } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ApiError } from '../lib/apiClient'
import { listProjects, type Project } from '../lib/projectsApi'

/**
 * documentation/09-ui-ux-design-system.md §5.6, Screen 3 — "Card grid. Each
 * card: name, repository, last scan time, risk score chip, verdict pill,
 * severity mini-bar. Empty state: illustration + 'Create your first
 * project'. Primary action top-right." The scan-derived fields (last scan,
 * risk score, verdict, severity mini-bar) aren't shown yet — nothing
 * produces them until Phase 6/8 — so each card shows what's real today:
 * name, repository, credential status, and creation date.
 */
export function ProjectsListPage() {
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
        if (cancelled) return
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load projects.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <main className="mx-auto max-w-6xl px-6 py-8">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-h1 text-text-primary">Projects</h1>
          <p className="text-body-sm text-text-secondary">
            Everything GuardPipe scans, one project per application or repository.
          </p>
        </div>
        <Button onClick={() => navigate('/projects/new')}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          New Project
        </Button>
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
          <FolderPlus className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
          <CardTitle>Create your first project</CardTitle>
          <CardDescription className="max-w-sm">
            A project is a container for one application or repository — attach a GitHub repository
            and GuardPipe scans it across every supply-chain stage.
          </CardDescription>
          <Button onClick={() => navigate('/projects/new')} className="mt-2">
            <Plus className="h-4 w-4" aria-hidden="true" />
            New Project
          </Button>
        </Card>
      )}

      {!error && projects !== null && projects.length > 0 && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => (
            <Card
              key={p.id}
              className="cursor-pointer transition-colors hover:border-accent/50"
              onClick={() => navigate(`/projects/${p.id}`)}
            >
              <div className="flex items-start justify-between gap-2">
                <CardTitle className="text-h3">{p.name}</CardTitle>
                <span
                  className={
                    p.status === 'active'
                      ? 'rounded-full bg-success/10 px-2 py-0.5 text-caption font-semibold text-success'
                      : 'rounded-full bg-bg-subtle px-2 py-0.5 text-caption font-semibold text-text-tertiary'
                  }
                >
                  {p.status}
                </span>
              </div>
              {p.description && (
                <CardDescription className="mt-1 line-clamp-2">{p.description}</CardDescription>
              )}
              <div className="mt-4 flex items-center gap-2 text-body-sm text-text-secondary">
                <FolderGit2 className="h-4 w-4 shrink-0" aria-hidden="true" />
                {p.repository ? (
                  <span className="truncate">
                    {p.repository.owner}/{p.repository.name}
                    {p.repository.is_private && (
                      <span className="ml-1.5 text-caption text-text-tertiary">(private)</span>
                    )}
                  </span>
                ) : (
                  <span className="text-text-tertiary">No repository attached</span>
                )}
              </div>
              {p.repository?.is_private && (
                <div className="mt-1 text-caption text-text-tertiary">
                  {p.has_credential ? 'Credential attached' : 'Credential required'}
                </div>
              )}
            </Card>
          ))}
        </div>
      )}
    </main>
  )
}
