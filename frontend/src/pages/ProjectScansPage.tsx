import { useEffect, useState } from 'react'
import { ScanSearch } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ScanHistoryTable } from '../components/project/ScanHistoryTable'
import { ScanLauncher } from '../components/project/ScanLauncher'
import { useProjectContext } from '../components/project/ProjectContext'
import { ApiError } from '../lib/apiClient'
import { listScans, type Scan, type ScanSummary } from '../lib/scansApi'

/**
 * The project's Scans tab — a launcher (pick engines, run one/several/all)
 * over the scan-history table: every past scan for this project (date/time,
 * type, status, finding counts), newest first, backed by
 * `GET /projects/{id}/scans`. Starting a scan lands on its live detail page
 * at `/scans/:id`.
 */
export function ProjectScansPage() {
  const { project, refetch } = useProjectContext()
  const navigate = useNavigate()
  const [scans, setScans] = useState<ScanSummary[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listScans(project.id)
      .then((res) => {
        if (!cancelled) setScans(res.data)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setLoadError(err instanceof ApiError ? err.problem.detail : 'Could not load past scans.')
      })
    return () => {
      cancelled = true
    }
  }, [project.id])

  function handleStarted(scan: Scan) {
    navigate(`/scans/${scan.id}`)
  }

  return (
    <main className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Scans</h1>
        <p className="text-body-sm text-text-secondary">
          Every scan run against this project, most recent first.
        </p>
      </div>

      <div className="mb-6">
        <ScanLauncher project={project} onStarted={handleStarted} onProjectRefresh={refetch} />
      </div>

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
            Run the first scan to see dependency, secret, and vulnerability findings for this
            project.
          </CardDescription>
        </Card>
      )}

      {!loadError && scans !== null && scans.length > 0 && <ScanHistoryTable scans={scans} />}
    </main>
  )
}
