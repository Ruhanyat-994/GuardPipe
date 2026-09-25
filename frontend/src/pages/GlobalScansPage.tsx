import { useEffect, useState } from 'react'
import { ScanSearch } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ScanHistoryTable } from '../components/project/ScanHistoryTable'
import { ScanLauncher } from '../components/project/ScanLauncher'
import { ApiError } from '../lib/apiClient'
import { listProjects, type Project } from '../lib/projectsApi'
import { listOrgScans, type OrgScanSummary, type Scan } from '../lib/scansApi'
import { cn } from '../lib/cn'
import { useActiveScansStore } from '../stores/activeScansStore'

/**
 * The global Scans page — run a scan against any existing project without
 * opening it first, and see the scan history across every project in one
 * table. The launcher itself (ScanLauncher — pick one, several, or all
 * engines) is unchanged from the per-project Scans tab; only the project
 * has to be chosen here first, since there's no ProjectContext to imply it.
 */
export function GlobalScansPage() {
  const navigate = useNavigate()
  const refreshActiveScans = useActiveScansStore((s) => s.refresh)
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [projectsError, setProjectsError] = useState<string | null>(null)
  const [selectedProjectId, setSelectedProjectId] = useState('')

  const [scans, setScans] = useState<OrgScanSummary[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)

  const selectedProject = projects?.find((p) => p.id === selectedProjectId) ?? null

  function loadProjects() {
    return listProjects()
      .then((res) => {
        setProjects(res.data)
        setSelectedProjectId((current) => current || (res.data.length > 0 ? res.data[0].id : ''))
      })
      .catch((err: unknown) => {
        setProjectsError(err instanceof ApiError ? err.problem.detail : 'Could not load projects.')
      })
  }

  useEffect(() => {
    void loadProjects()
  }, [])

  useEffect(() => {
    let cancelled = false
    listOrgScans()
      .then((res) => {
        if (!cancelled) setScans(res.data)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setLoadError(
            err instanceof ApiError ? err.problem.detail : 'Could not load scan history.',
          )
      })
    return () => {
      cancelled = true
    }
  }, [])

  function handleStarted(scan: Scan) {
    refreshActiveScans()
    navigate(`/scans/${scan.id}`)
  }

  return (
    <main className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Scans</h1>
        <p className="text-body-sm text-text-secondary">
          Run a scan against any existing project, and see every scan across every project below.
        </p>
      </div>

      {projectsError && (
        <Card className="mb-6 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {projectsError}
          </p>
        </Card>
      )}

      {!projectsError && projects === null && (
        <p className="mb-6 text-body-sm text-text-secondary">Loading projects…</p>
      )}

      {!projectsError && projects !== null && projects.length === 0 && (
        <Card className="mb-6 flex flex-col items-center gap-3 py-16 text-center">
          <ScanSearch className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
          <CardTitle>No projects yet</CardTitle>
          <CardDescription className="max-w-sm">
            Create a project before running a scan.
          </CardDescription>
          <Button onClick={() => navigate('/projects/new')} className="mt-2">
            New Project
          </Button>
        </Card>
      )}

      {!projectsError && projects !== null && projects.length > 0 && (
        <div className="mb-6 flex flex-col gap-4">
          <Card>
            <CardTitle className="text-h3">Choose a project</CardTitle>
            <CardDescription className="mt-1">Pick which existing project to scan.</CardDescription>
            <label className="mt-3 flex items-center gap-2 text-body-sm text-text-secondary">
              Project
              <select
                value={selectedProjectId}
                onChange={(e) => setSelectedProjectId(e.target.value)}
                className={cn(
                  'h-9 flex-1 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary',
                  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2',
                )}
              >
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
          </Card>

          {selectedProject && (
            <ScanLauncher
              project={selectedProject}
              onStarted={handleStarted}
              onProjectRefresh={() => void loadProjects()}
            />
          )}
        </div>
      )}

      <h2 className="mb-3 text-h3 text-text-primary">History</h2>

      {loadError && (
        <Card className="border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {loadError}
          </p>
        </Card>
      )}

      {!loadError && scans === null && (
        <p className="text-body-sm text-text-secondary">Loading scan history…</p>
      )}

      {!loadError && scans !== null && scans.length === 0 && (
        <Card className="flex flex-col items-center gap-3 py-16 text-center">
          <ScanSearch className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
          <CardTitle>No scans yet</CardTitle>
          <CardDescription className="max-w-sm">
            Run a scan above to see it appear here, across every project.
          </CardDescription>
        </Card>
      )}

      {!loadError && scans !== null && scans.length > 0 && (
        <ScanHistoryTable scans={scans} showProject />
      )}
    </main>
  )
}
