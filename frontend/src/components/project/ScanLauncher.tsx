import { useState } from 'react'
import { CheckSquare, PlayCircle, Square } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { cn } from '../../lib/cn'
import { ALL_ENGINES, ENABLED_ENGINES, ENGINE_META, isEngineEnabled } from '../../lib/engines'
import type { Engine } from '../../lib/rulesApi'
import { ApiError } from '../../lib/apiClient'
import { createScan, type Scan } from '../../lib/scansApi'
import { OsvMark } from '../icons/OsvMark'
import { SonarQubeMark } from '../icons/SonarQubeMark'

/**
 * The scan launcher — replaces the single "Run Scan" button with an
 * explicit choice: every one of the seven engines is shown (never hide the
 * pipeline's true shape), only the ones actually registered on the backend
 * (lib/engines.ts's ENABLED_ENGINES) are selectable, and two
 * distinct actions cover both things a user asked for — run everything
 * available (`type: full_supply_chain`), or run exactly the engines picked
 * (`type: partial`, one or several). One selected engine is exactly "run
 * that scan only"; several is "run these scans."
 */
export function ScanLauncher({
  projectId,
  onStarted,
}: {
  projectId: string
  onStarted: (scan: Scan) => void
}) {
  const [selected, setSelected] = useState<Set<Engine>>(() => new Set(ENABLED_ENGINES))
  const [starting, setStarting] = useState<'all' | 'selected' | null>(null)
  const [error, setError] = useState<string | null>(null)

  function toggle(engine: Engine) {
    if (!isEngineEnabled(engine)) return
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(engine)) {
        next.delete(engine)
      } else {
        next.add(engine)
      }
      return next
    })
  }

  async function runAll() {
    setStarting('all')
    setError(null)
    try {
      onStarted(await createScan(projectId, { type: 'full_supply_chain' }))
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not start the scan.')
      setStarting(null)
    }
  }

  async function runSelected() {
    if (selected.size === 0) return
    setStarting('selected')
    setError(null)
    try {
      onStarted(await createScan(projectId, { type: 'partial', engines: Array.from(selected) }))
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not start the scan.')
      setStarting(null)
    }
  }

  return (
    <Card>
      <CardTitle>Run a scan</CardTitle>
      <CardDescription className="mt-1">
        Pick one or more engines to run, or run everything available. Engines still greyed out
        haven't been built yet and will join automatically once they land.
      </CardDescription>

      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        {ALL_ENGINES.map((engine) => {
          const meta = ENGINE_META[engine]
          const Icon = meta.icon
          const enabled = isEngineEnabled(engine)
          const isSelected = enabled && selected.has(engine)
          const CheckIcon = isSelected ? CheckSquare : Square
          return (
            <button
              key={engine}
              type="button"
              disabled={!enabled}
              aria-pressed={isSelected}
              onClick={() => toggle(engine)}
              title={enabled ? undefined : 'Not built yet — lands in a later phase'}
              className={cn(
                'flex flex-col items-center gap-2 rounded-lg border px-3 py-3 text-center transition-colors',
                !enabled && 'cursor-not-allowed opacity-45',
                enabled && isSelected && 'border-accent bg-accent/5',
                enabled &&
                  !isSelected &&
                  'border-border-default bg-bg-surface hover:border-border-strong',
                !enabled && 'border-border-default bg-bg-subtle',
              )}
            >
              <div className="flex w-full items-center justify-between">
                <CheckIcon
                  className={cn('h-4 w-4', isSelected ? 'text-accent' : 'text-text-tertiary')}
                  aria-hidden="true"
                />
                <Icon className="h-5 w-5 text-text-secondary" aria-hidden="true" />
              </div>
              <div className="flex items-center gap-1">
                <span className="text-body-sm font-medium text-text-primary">{meta.label}</span>
                {meta.hasOsvMark && <OsvMark />}
                {meta.hasSonarQubeMark && <SonarQubeMark />}
              </div>
              <span className="text-caption text-text-tertiary">
                {enabled ? 'Available' : 'Coming soon'}
              </span>
            </button>
          )
        })}
      </div>

      {error && (
        <p role="alert" className="mt-4 text-body-sm text-danger">
          {error}
        </p>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <Button onClick={() => void runAll()} loading={starting === 'all'}>
          <PlayCircle className="h-4 w-4" aria-hidden="true" />
          Run All Scans
        </Button>
        <Button
          variant="secondary"
          onClick={() => void runSelected()}
          loading={starting === 'selected'}
          disabled={selected.size === 0}
        >
          Run Selected ({selected.size})
        </Button>
      </div>
    </Card>
  )
}
