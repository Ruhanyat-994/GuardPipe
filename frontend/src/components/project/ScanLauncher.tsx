import { useEffect, useState } from 'react'
import { CheckSquare, PlayCircle, ShieldCheck, Square } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { cn } from '../../lib/cn'
import {
  ALL_ENGINES,
  engineRequirementReason,
  ENGINE_META,
  isEngineEnabled,
  isEngineRunnable,
} from '../../lib/engines'
import type { Engine } from '../../lib/rulesApi'
import { ApiError } from '../../lib/apiClient'
import {
  ATTESTATION_STATEMENT,
  attestTarget,
  listTargets,
  type Project,
  type Target,
} from '../../lib/projectsApi'
import { createScan, type PentestConfigInput, type Scan } from '../../lib/scansApi'
import { DocumentUploadForm } from './DocumentUploadForm'
import { PentestOptionsPanel } from './PentestOptionsPanel'
import { GeminiMark } from '../icons/GeminiMark'
import { GitHubMark } from '../icons/GitHubMark'
import { KubernetesMark } from '../icons/KubernetesMark'
import { OsvMark } from '../icons/OsvMark'
import { SonarQubeMark } from '../icons/SonarQubeMark'

/**
 * The scan launcher — replaces the single "Run Scan" button with an
 * explicit choice: every one of the seven engines is shown (never hide the
 * pipeline's true shape), only the ones that can actually run for this
 * project — registered on the backend (lib/engines.ts's ENABLED_ENGINES)
 * *and* structurally applicable (a repo-based engine needs a repository
 * attached, pentest needs an attested target, `isEngineRunnable`) — are
 * selectable. Two distinct actions cover both things a user asked for — run
 * everything available (`type: full_supply_chain`), or run exactly the
 * engines picked (`type: partial`, one or several). One selected engine is
 * exactly "run that scan only"; several is "run these scans."
 *
 * Pentest additionally needs its target's authorisation attestation
 * accepted before it can run at all (FR-PEN-001/NFR-CMP-001) — a one-time
 * act on the *target*, not the scan, so if this project's target is still
 * `awaiting_attestation` this component surfaces that inline instead of
 * sending the user to the Targets tab first.
 */
export function ScanLauncher({
  project,
  onStarted,
  onProjectRefresh,
}: {
  project: Project
  onStarted: (scan: Scan) => void
  // Called after a successful attestation so the caller can refresh its own
  // copy of `project` (ProjectScansPage has a ProjectContext to refetch;
  // GlobalScansPage doesn't — no context to depend on here, so this stays a
  // plain optional prop rather than reaching for context directly).
  onProjectRefresh?: () => void
}) {
  const [selected, setSelected] = useState<Set<Engine>>(
    () => new Set(ALL_ENGINES.filter((e) => isEngineRunnable(e, project))),
  )
  const [starting, setStarting] = useState<'all' | 'selected' | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Empty object, not undefined — an untouched panel still resolves
  // server-side to a real config (the literal Stealth default), so "the
  // client configured nothing" and "the client explicitly picked Stealth"
  // are the same request body, matching CreateScanInput's own doc comment.
  const [pentestConfig, setPentestConfig] = useState<PentestConfigInput>({})

  // Only relevant while pentest is enabled but this project has no attested
  // target yet — fetched lazily so a project that already has one (or never
  // will) never pays for the extra round-trip.
  const [pendingTarget, setPendingTarget] = useState<Target | null>(null)
  const [targetsChecked, setTargetsChecked] = useState(false)
  const [attestChecked, setAttestChecked] = useState(false)
  const [attesting, setAttesting] = useState(false)
  const [attestError, setAttestError] = useState<string | null>(null)

  useEffect(() => {
    if (!isEngineEnabled('pentest') || project.has_pentest_target || targetsChecked) return
    let cancelled = false
    listTargets(project.id)
      .then((res) => {
        if (cancelled) return
        setPendingTarget(res.data.find((t) => t.status === 'awaiting_attestation') ?? null)
      })
      .finally(() => {
        if (!cancelled) setTargetsChecked(true)
      })
    return () => {
      cancelled = true
    }
  }, [project.has_pentest_target, project.id, targetsChecked])

  // Pentest is a special case for *selectability*: it must be pickable the
  // moment a target exists and is merely awaiting attestation (picking it
  // is what surfaces the attestation panel below), not only once it's
  // already attested — `isEngineRunnable` alone would keep the checkbox
  // stuck disabled forever, since attesting is exactly the thing this
  // component lets the user do. Every other engine still uses the plain
  // repo/target-presence gate.
  function isSelectable(engine: Engine): boolean {
    if (engine === 'pentest') {
      return isEngineEnabled('pentest') && (!!project.has_pentest_target || !!pendingTarget)
    }
    return isEngineRunnable(engine, project)
  }

  function toggle(engine: Engine) {
    if (!isSelectable(engine)) return
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

  async function attestPendingTarget(): Promise<boolean> {
    if (!pendingTarget) return true
    setAttestError(null)
    setAttesting(true)
    try {
      await attestTarget(pendingTarget.id)
      setPendingTarget(null)
      onProjectRefresh?.()
      return true
    } catch (err) {
      setAttestError(err instanceof ApiError ? err.problem.detail : 'Could not record attestation.')
      return false
    } finally {
      setAttesting(false)
    }
  }

  // `full_supply_chain` means "everything applicable" — the backend itself
  // already excludes pentest from that set when no attestation exists
  // (internal/modules/orchestrator/service.go's resolveEngines), so "Run
  // All" never needs to gate on attestation here; it simply runs one fewer
  // engine until the target is attested via the panel below or the Targets
  // tab.
  async function runAll() {
    setStarting('all')
    setError(null)
    try {
      onStarted(
        await createScan(project.id, {
          type: 'full_supply_chain',
          ...(isSelectable('pentest') ? { pentest_config: pentestConfig } : {}),
        }),
      )
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not start the scan.')
      setStarting(null)
    }
  }

  // `partial` sends an explicit engine list the backend validates strictly
  // (a structurally-inapplicable engine is rejected, not silently dropped)
  // — so if the user explicitly checked pentest while it's still awaiting
  // attestation, attest first and only then send the request.
  async function runSelected() {
    if (selected.size === 0) return
    if (selected.has('pentest') && !project.has_pentest_target && pendingTarget) {
      if (!attestChecked) return
      if (!(await attestPendingTarget())) return
    }
    setStarting('selected')
    setError(null)
    try {
      onStarted(
        await createScan(project.id, {
          type: 'partial',
          engines: Array.from(selected),
          ...(selected.has('pentest') ? { pentest_config: pentestConfig } : {}),
        }),
      )
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not start the scan.')
      setStarting(null)
    }
  }

  const runSelectedNeedsAttestation =
    selected.has('pentest') && !project.has_pentest_target && !!pendingTarget
  const runSelectedBlocked = runSelectedNeedsAttestation && !attestChecked

  return (
    <Card>
      <CardTitle>Run a scan</CardTitle>
      <CardDescription className="mt-1">
        Pick one or more engines to run, or run everything available. A greyed-out engine either
        hasn&rsquo;t been built yet, or needs something this project doesn&rsquo;t have yet — a
        repository, or an attested pentest target — hover it to see which.
      </CardDescription>

      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        {ALL_ENGINES.map((engine) => {
          const meta = ENGINE_META[engine]
          const Icon = meta.icon
          const enabled = isEngineEnabled(engine)
          const selectable = isSelectable(engine)
          const isSelected = selectable && selected.has(engine)
          const CheckIcon = isSelected ? CheckSquare : Square
          const awaitingAttestation =
            engine === 'pentest' && !project.has_pentest_target && !!pendingTarget
          const caption = !enabled
            ? 'Coming soon'
            : awaitingAttestation
              ? 'Needs attestation'
              : !selectable
                ? (engineRequirementReason(engine, project) ?? 'Unavailable')
                : 'Available'
          const title = !enabled
            ? 'Not built yet — lands in a later phase'
            : awaitingAttestation
              ? 'Select this to accept the authorisation attestation before running'
              : !selectable
                ? (engineRequirementReason(engine, project) ?? undefined)
                : undefined
          return (
            <button
              key={engine}
              type="button"
              disabled={!selectable}
              aria-pressed={isSelected}
              onClick={() => toggle(engine)}
              title={title}
              className={cn(
                'flex flex-col items-center gap-2 rounded-lg border px-3 py-3 text-center transition-colors',
                !selectable && 'cursor-not-allowed opacity-45',
                selectable && isSelected && 'border-accent bg-accent/5',
                selectable &&
                  !isSelected &&
                  'border-border-default bg-bg-surface hover:border-border-strong',
                !selectable && 'border-border-default bg-bg-subtle',
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
                {meta.hasKubernetesMark && <KubernetesMark />}
                {meta.hasGitHubMark && <GitHubMark className="h-3 w-3" />}
                {meta.hasGeminiMark && <GeminiMark />}
              </div>
              <span className="text-caption text-text-tertiary">{caption}</span>
            </button>
          )
        })}
      </div>

      {selected.has('docreview') && isEngineEnabled('docreview') && (
        <DocumentUploadForm projectId={project.id} variant="embedded" />
      )}

      {selected.has('pentest') && isSelectable('pentest') && (
        <PentestOptionsPanel value={pentestConfig} onChange={setPentestConfig} />
      )}

      {/* Mandatory authorisation-attestation gate (FR-PEN-001/NFR-CMP-001) —
          appears only once pentest is checked for an explicit partial run
          and its target hasn't been attested yet. A weighty, hard-to-miss
          block since this is a legal record, matching
          documentation/09-ui-ux-design-system.md §5.5/§5.7's own framing. */}
      {runSelectedNeedsAttestation && (
        <div className="mt-4 rounded-lg border border-warning/40 bg-warning/5 p-4">
          <div className="flex items-start gap-2">
            <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <div>
              <p className="text-body-sm font-semibold text-text-primary">
                Authorisation required before Pentest can run
              </p>
              <p className="mt-1 text-body-sm text-text-secondary">{ATTESTATION_STATEMENT}</p>
            </div>
          </div>
          <label className="mt-3 flex items-center gap-2 text-body-sm text-text-primary">
            <input
              type="checkbox"
              checked={attestChecked}
              onChange={(e) => setAttestChecked(e.target.checked)}
              className="h-4 w-4 rounded border-border-strong accent-accent"
            />
            I confirm I own or am explicitly authorised to test this target.
          </label>
          {attestError && (
            <p role="alert" className="mt-2 text-body-sm text-danger">
              {attestError}
            </p>
          )}
        </div>
      )}

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
          loading={starting === 'selected' || attesting}
          disabled={selected.size === 0 || runSelectedBlocked}
        >
          Run Selected ({selected.size})
        </Button>
      </div>
    </Card>
  )
}
