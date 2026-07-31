import { useEffect, useState } from 'react'
import { Crosshair } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { TargetRegisterForm } from '../components/project/TargetRegisterForm'
import { useProjectContext } from '../components/project/ProjectContext'
import { ApiError } from '../lib/apiClient'
import { listTargets, type Target } from '../lib/projectsApi'
import { cn } from '../lib/cn'

const STATUS_STYLES: Record<Target['status'], string> = {
  awaiting_attestation: 'bg-warning/10 text-warning',
  attested: 'bg-success/10 text-success',
  blocked: 'bg-danger/10 text-danger',
  revoked: 'bg-bg-subtle text-text-tertiary',
}

/** FR-PRJ-006/007 — real data, unlike the Scans/Findings tabs, since the
 * target registration/validation backend has existed since Phase 3. */
export function ProjectTargetsPage() {
  const { project } = useProjectContext()
  const [targets, setTargets] = useState<Target[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  function refresh() {
    listTargets(project.id)
      .then((res) => setTargets(res.data))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load targets.')
      })
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project.id])

  return (
    <main className="mx-auto max-w-3xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">Pentest targets</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        Hosts or URLs GuardPipe may later probe, once you accept the authorisation attestation (that
        UI itself lands with the pentest engine, Phase 7).
      </p>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      <Card className="mb-4">
        <CardTitle className="text-h3">Registered targets</CardTitle>
        {targets === null && !error && <CardDescription className="mt-2">Loading…</CardDescription>}
        {targets !== null && targets.length === 0 && (
          <div className="mt-4 flex flex-col items-center gap-2 py-8 text-center">
            <Crosshair className="h-8 w-8 text-text-tertiary" aria-hidden="true" />
            <CardDescription>No targets registered yet.</CardDescription>
          </div>
        )}
        {targets !== null && targets.length > 0 && (
          <ul className="mt-4 flex flex-col divide-y divide-border-default">
            {targets.map((t) => (
              <li key={t.id} className="flex items-center justify-between gap-3 py-3">
                <div className="min-w-0">
                  <div className="truncate text-body-sm text-text-primary">{t.normalized_host}</div>
                  <div className="truncate text-caption text-text-tertiary">
                    {t.pinned_ips.join(', ') || 'no resolved address'}
                  </div>
                </div>
                <span
                  className={cn(
                    'shrink-0 rounded-full px-2 py-0.5 text-caption font-semibold capitalize',
                    STATUS_STYLES[t.status],
                  )}
                >
                  {t.status.replace('_', ' ')}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <TargetRegisterForm projectId={project.id} onRegistered={refresh} />
    </main>
  )
}
