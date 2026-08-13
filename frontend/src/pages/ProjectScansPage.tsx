import { useState } from 'react'
import { PlayCircle } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { useProjectContext } from '../components/project/ProjectContext'
import { ApiError } from '../lib/apiClient'
import { createScan } from '../lib/scansApi'

/**
 * The project's Scans tab (BUILD_GUIDE.md Phase 6) — no "list past scans"
 * endpoint exists yet (documentation/07-api-specification.md §5 has no
 * `GET /projects/{id}/scans`, only creation + per-scan lookup), so this tab
 * is the trigger: start a scan, land on its live detail page at
 * `/scans/:id`. A scan history table is a later-phase addition once that
 * endpoint exists.
 */
export function ProjectScansPage() {
  const { project } = useProjectContext()
  const navigate = useNavigate()
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function runScan() {
    setStarting(true)
    setError(null)
    try {
      const scan = await createScan(project.id)
      navigate(`/scans/${scan.id}`)
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not start the scan.')
      setStarting(false)
    }
  }

  return (
    <main className="mx-auto max-w-2xl px-6 py-16">
      <Card className="flex flex-col items-center gap-3 text-center">
        <PlayCircle className="h-8 w-8 text-accent" aria-hidden="true" />
        <CardTitle>Run a scan</CardTitle>
        <CardDescription>
          Scans depscan's manifests and lockfiles for known-vulnerable dependencies, missing
          lockfiles, and committed secrets. The other six engines land in later phases and will
          show as "not run" on the execution graph until then.
        </CardDescription>
        {error && (
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        )}
        <Button onClick={() => void runScan()} loading={starting}>
          Run Scan
        </Button>
      </Card>
    </main>
  )
}
