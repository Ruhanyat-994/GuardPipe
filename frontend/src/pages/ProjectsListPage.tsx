import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Archive,
  FolderPlus,
  GitBranch,
  KeyRound,
  Plus,
  Settings as SettingsIcon,
} from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ContextMenu } from '../components/ContextMenu'
import { GitHubMark } from '../components/icons/GitHubMark'
import { ApiError } from '../lib/apiClient'
import { archiveProject, listProjects, type Project } from '../lib/projectsApi'
import { relativeTime } from '../lib/format'

/**
 * documentation/09-ui-ux-design-system.md §5.6, Screen 3 — "Card grid. Each
 * card: name, repository, last scan time, risk score chip, verdict pill,
 * severity mini-bar. Empty state: illustration + 'Create your first
 * project'. Primary action top-right." The scan-derived fields (last scan,
 * risk score, verdict, severity mini-bar) aren't shown yet — nothing
 * produces them until Phase 6/8 — so each card shows what's real today:
 * name, repository (with its provider's own brand mark, §4.5), default
 * branch, credential status, creation date, and a per-card `ContextMenu`
 * for Settings/Archive without opening the project.
 */
export function ProjectsListPage() {
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  function refresh() {
    listProjects()
      .then((res) => setProjects(res.data))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load projects.')
      })
  }

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

  async function handleArchive(p: Project) {
    if (!window.confirm(`Archive "${p.name}"?`)) return
    await archiveProject(p.id)
    refresh()
  }

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
              role="button"
              tabIndex={0}
              className="cursor-pointer transition-all hover:-translate-y-0.5 hover:border-accent/50 hover:shadow-md"
              onClick={() => navigate(`/projects/${p.id}`)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') navigate(`/projects/${p.id}`)
              }}
            >
              <div className="flex items-start justify-between gap-2">
                <CardTitle className="text-h3">{p.name}</CardTitle>
                <div className="flex shrink-0 items-center gap-1.5">
                  <span
                    className={
                      p.status === 'active'
                        ? 'rounded-full bg-success/10 px-2 py-0.5 text-caption font-semibold text-success'
                        : 'rounded-full bg-bg-subtle px-2 py-0.5 text-caption font-semibold text-text-tertiary'
                    }
                  >
                    {p.status}
                  </span>
                  {/* Stop the card's own onClick from also firing when the
                      menu (or one of its items) is used. */}
                  <div onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
                    <ContextMenu
                      label={`${p.name} actions`}
                      groups={[
                        [
                          {
                            label: 'Settings',
                            icon: SettingsIcon,
                            onClick: () => navigate(`/projects/${p.id}/settings`),
                          },
                        ],
                        [
                          {
                            label: 'Archive project',
                            icon: Archive,
                            destructive: true,
                            onClick: () => void handleArchive(p),
                          },
                        ],
                      ]}
                    />
                  </div>
                </div>
              </div>
              {p.description && (
                <CardDescription className="mt-1 line-clamp-2">{p.description}</CardDescription>
              )}

              <div className="mt-4 flex items-center gap-2 text-body-sm text-text-secondary">
                {p.repository ? (
                  <>
                    <GitHubMark className="h-4 w-4 shrink-0" />
                    <span className="truncate">
                      {p.repository.owner}/{p.repository.name}
                    </span>
                    {p.repository.is_private && (
                      <span className="shrink-0 rounded-full bg-bg-subtle px-1.5 py-0.5 text-caption text-text-tertiary">
                        Private
                      </span>
                    )}
                  </>
                ) : (
                  <span className="text-text-tertiary">No repository attached</span>
                )}
              </div>

              {p.repository && (
                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-caption text-text-tertiary">
                  <span className="flex items-center gap-1">
                    <GitBranch className="h-3.5 w-3.5" aria-hidden="true" />
                    {p.repository.default_branch}
                  </span>
                  {p.repository.is_private && (
                    <span className="flex items-center gap-1">
                      <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
                      {p.has_credential ? 'Credential attached' : 'Credential required'}
                    </span>
                  )}
                </div>
              )}

              <div className="mt-3 text-caption text-text-tertiary">
                Created {relativeTime(p.created_at)}
              </div>
            </Card>
          ))}
        </div>
      )}
    </main>
  )
}
