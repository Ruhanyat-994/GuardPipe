import { useCallback, useEffect, useState } from 'react'
import { Outlet, useParams } from 'react-router-dom'
import { ProjectContext } from '../components/project/ProjectContext'
import { ProjectTabBar } from '../components/project/ProjectTabBar'
import { ApiError } from '../lib/apiClient'
import { getProject, type Project } from '../lib/projectsApi'

/**
 * Layout route for every `/projects/:id/*` screen — fetches the project
 * once, renders `ProjectTabBar`, and hands the project down to whichever
 * tab is active via `ProjectContext` so Overview/Targets/Settings don't
 * each re-fetch it.
 */
export function ProjectLayout() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Exposed via ProjectContext for child tabs to call after a mutation
  // (e.g. saving Settings) — not itself called from an effect, so it's
  // fine for it to setState directly.
  const refetch = useCallback(() => {
    if (!id) return
    setError(null)
    getProject(id)
      .then(setProject)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load this project.')
      })
  }, [id])

  useEffect(() => {
    if (!id) return
    let cancelled = false
    getProject(id)
      .then((p) => {
        if (!cancelled) setProject(p)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load this project.')
        }
      })
    return () => {
      cancelled = true
    }
  }, [id])

  if (error) {
    return (
      <main className="mx-auto max-w-2xl px-6 py-8">
        <p role="alert" className="text-body-sm text-danger">
          {error}
        </p>
      </main>
    )
  }

  if (!project) {
    return (
      <main className="mx-auto max-w-6xl px-6 py-8">
        <p className="text-body-sm text-text-secondary">Loading project…</p>
      </main>
    )
  }

  return (
    <ProjectContext.Provider value={{ project, refetch }}>
      <ProjectTabBar project={project} />
      <Outlet />
    </ProjectContext.Provider>
  )
}
