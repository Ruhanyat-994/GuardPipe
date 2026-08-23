import { useState } from 'react'
import {
  CheckCircle2,
  Circle,
  FileText,
  GitBranch,
  Loader2,
  MinusCircle,
  Package,
  ShieldAlert,
  XCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '../../lib/cn'
import { engineRequirementReason, ENGINE_META, isEngineRunnable } from '../../lib/engines'
import type { Project } from '../../lib/projectsApi'
import type { Engine } from '../../lib/rulesApi'
import type { Job, JobStatus, Progress, Scan } from '../../lib/scansApi'
import { EngineRunDetail } from './EngineRunDetail'
import { ScanProgressBar } from './ScanProgressBar'
import { GeminiMark } from '../icons/GeminiMark'
import { GitHubMark } from '../icons/GitHubMark'
import { KubernetesMark } from '../icons/KubernetesMark'
import { OsvMark } from '../icons/OsvMark'
import { SonarQubeMark } from '../icons/SonarQubeMark'

/**
 * The live scan execution graph (documentation/09-ui-ux-design-system.md
 * §4.8) — the orchestrator's own execution DAG (workspace prep fans out to
 * six parallel engines; pentest branches independently straight off scan
 * start since it needs no workspace; all seven converge into ai enrichment
 * -> scoring), rendered as connected nodes rather than the flat "seven
 * cards" it replaces. Driven entirely by `GET /scans/{id}/progress`,
 * polled every 2s (FR-UI-002) — no new backend data needed.
 *
 * Every node in the fixed shape renders even when its engine hasn't run —
 * Phase 6 only registers depscan, so every other engine node legitimately
 * shows `not_run` this phase. That's the true state (they didn't run),
 * not a placeholder standing in for missing backend work — exactly the
 * distinction this design system's "never fake it" rule cares about
 * (GlobalSearch/NotificationPanel, documentation/09 §4.4). For the same
 * reason, the header only surfaces fields the scan actually has (id, type,
 * branch, queued time, status) — no fabricated "started by"/commit/CI-run
 * chrome the backend doesn't produce.
 */

type NodeStatus = 'not_run' | 'running' | 'succeeded' | 'failed' | 'skipped'

const STATUS_COLOR: Record<NodeStatus, string> = {
  not_run: 'var(--text-tertiary)',
  running: 'var(--accent)',
  succeeded: 'var(--success)',
  failed: 'var(--danger)',
  skipped: 'var(--text-tertiary)',
}

const STATUS_LABEL: Record<NodeStatus, string> = {
  not_run: 'Not run',
  running: 'In progress',
  succeeded: 'Succeeded',
  failed: 'Failed',
  skipped: 'Skipped',
}

const STATUS_ICON: Record<NodeStatus, LucideIcon> = {
  not_run: Circle,
  running: Loader2,
  succeeded: CheckCircle2,
  failed: XCircle,
  skipped: MinusCircle,
}

const PARALLEL_ENGINES: Engine[] = [
  'docreview',
  'codescan',
  'depscan',
  'containerscan',
  'k8sscan',
  'cicdscan',
]

function jobStatusToNodeStatus(status: JobStatus): NodeStatus {
  switch (status) {
    case 'queued':
      return 'not_run'
    case 'running':
      return 'running'
    case 'succeeded':
      return 'succeeded'
    case 'failed':
      return 'failed'
    case 'skipped':
    case 'cancelled':
      return 'skipped'
  }
}

interface EngineNodeState {
  status: NodeStatus
  findingCount: number
  reason: string | null
}

function resolveEngineState(
  engine: Engine,
  progress: Progress | null,
  jobs: Job[],
): EngineNodeState {
  const live = progress?.engines.find((e) => e.engine === engine)
  if (live) {
    return {
      status: jobStatusToNodeStatus(live.status),
      findingCount: live.finding_count,
      reason: null,
    }
  }
  const job = jobs.find((j) => j.engine === engine)
  if (job) {
    return {
      status: jobStatusToNodeStatus(job.status),
      findingCount: job.finding_count,
      reason: job.error_reason ?? job.skip_reason,
    }
  }
  return { status: 'not_run', findingCount: 0, reason: null }
}

function StatusBadge({ status, size = 'md' }: { status: NodeStatus; size?: 'sm' | 'md' }) {
  const color = STATUS_COLOR[status]
  const Icon = STATUS_ICON[status]
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full font-semibold capitalize',
        size === 'md' ? 'px-3 py-1 text-body-sm' : 'text-caption',
      )}
      style={{
        color,
        backgroundColor:
          size === 'md' ? `color-mix(in srgb, ${color} 12%, transparent)` : undefined,
      }}
    >
      <Icon
        className={cn('h-3.5 w-3.5', status === 'running' && 'animate-spin')}
        aria-hidden="true"
      />
      {STATUS_LABEL[status]}
    </span>
  )
}

function Node({
  label,
  icon: Icon,
  status,
  osvMark,
  sonarQubeMark,
  kubernetesMark,
  githubMark,
  geminiMark,
  tooltip,
  onClick,
  selected,
}: {
  label: string
  icon: LucideIcon
  status: NodeStatus
  osvMark?: boolean
  sonarQubeMark?: boolean
  kubernetesMark?: boolean
  githubMark?: boolean
  geminiMark?: boolean
  tooltip?: string | null
  onClick?: () => void
  selected?: boolean
}) {
  const color = STATUS_COLOR[status]
  const StatusIcon = STATUS_ICON[status]
  const Tag = onClick ? 'button' : 'div'
  return (
    <Tag
      type={onClick ? 'button' : undefined}
      onClick={onClick}
      aria-pressed={onClick ? selected : undefined}
      className={cn(
        'relative z-[1] flex w-[116px] shrink-0 flex-col items-center gap-2 rounded-xl border bg-bg-surface px-3 py-3.5 text-center shadow-sm transition-colors',
        onClick && 'cursor-pointer hover:border-accent/50 hover:shadow-md',
      )}
      style={{
        borderColor: selected
          ? 'var(--accent)'
          : status === 'not_run'
            ? 'var(--border-default)'
            : `color-mix(in srgb, ${color} 45%, transparent)`,
        boxShadow: selected
          ? '0 0 0 2px color-mix(in srgb, var(--accent) 30%, transparent)'
          : undefined,
      }}
      title={tooltip ?? undefined}
    >
      <div
        className={cn('flex h-10 w-10 items-center justify-center rounded-full')}
        style={{ backgroundColor: `color-mix(in srgb, ${color} 15%, transparent)`, color }}
      >
        <Icon className="h-5 w-5" aria-hidden="true" />
      </div>
      <div className="flex items-center gap-1">
        <span className="text-body-sm font-semibold text-text-primary">{label}</span>
        {osvMark && <OsvMark />}
        {sonarQubeMark && <SonarQubeMark />}
        {kubernetesMark && <KubernetesMark />}
        {githubMark && <GitHubMark className="h-3 w-3" />}
        {geminiMark && <GeminiMark />}
      </div>
      <span
        className="flex items-center gap-1 text-caption font-medium capitalize"
        style={{ color }}
      >
        <StatusIcon
          className={cn('h-3 w-3', status === 'running' && 'animate-spin')}
          aria-hidden="true"
        />
        {STATUS_LABEL[status]}
      </span>
    </Tag>
  )
}

/** A short horizontal connector between two stages in the left-to-right
 * flow, with a joint dot at its midpoint — GitHub Actions' own connector
 * style (documentation/09-ui-ux-design-system.md §4.8's reference), and
 * the reason the whole graph now reads as wide-and-shallow instead of the
 * tall narrow column the vertical version produced. */
function HConnector() {
  return (
    <div className="flex w-10 shrink-0 items-center" aria-hidden="true">
      <div className="h-px flex-1 bg-border-default" />
      <div className="h-1.5 w-1.5 shrink-0 rounded-full bg-border-strong" />
      <div className="h-px flex-1 bg-border-default" />
    </div>
  )
}

function overallStatus(scanStarted: boolean, progress: Progress | null, jobs: Job[]): NodeStatus {
  if (!scanStarted) return 'not_run'
  if (progress) {
    switch (progress.status) {
      case 'queued':
        return 'not_run'
      case 'running':
        return 'running'
      case 'completed':
        return 'succeeded'
      case 'failed':
        return 'failed'
      case 'cancelled':
        return 'skipped'
    }
  }
  if (jobs.some((j) => j.status === 'failed')) return 'failed'
  if (jobs.every((j) => j.status !== 'queued' && j.status !== 'running')) return 'succeeded'
  return 'running'
}

const LEGEND: { status: NodeStatus; label: string }[] = [
  { status: 'succeeded', label: 'Succeeded' },
  { status: 'not_run', label: 'Not run' },
  { status: 'running', label: 'In progress' },
  { status: 'failed', label: 'Failed' },
  { status: 'skipped', label: 'Skipped' },
]

export function SupplyChainPipeline({
  progress,
  jobs,
  scan,
  project,
}: {
  progress: Progress | null
  jobs: Job[]
  scan?: Pick<Scan, 'id' | 'type' | 'branch' | 'queued_at'>
  // Optional — a caller without the project handy yet (e.g. mid-fetch) just
  // gets no repo/target-based disabling this render; every node still shows
  // its real job status either way.
  project?: Pick<Project, 'repository' | 'has_pentest_target'> | null
}) {
  const scanStarted = progress !== null || jobs.length > 0
  const anyRunningOrDone =
    jobs.some((j) => j.status !== 'queued') || (progress?.progress_pct ?? 0) > 0
  const workspaceStatus: NodeStatus = !scanStarted
    ? 'not_run'
    : anyRunningOrDone
      ? 'succeeded'
      : 'running'

  const pentestState = resolveEngineState('pentest', progress, jobs)
  const status = overallStatus(scanStarted, progress, jobs)

  // Only nodes with a real per-engine job to drill into are clickable —
  // "Scan start"/"Workspace prep"/"AI enrichment"/"Scoring" have no job of
  // their own (workspace prep is shared, the other two don't exist as
  // engines yet), so making them clickable would mean either faking data
  // for them or opening a panel that just says "nothing here," neither of
  // which earns a click. Same `isEngineEnabled` gate ScanLauncher already
  // uses for "can this engine even run today."
  const [selectedEngine, setSelectedEngine] = useState<Engine | null>(null)
  function toggleEngine(engine: Engine) {
    setSelectedEngine((current) => (current === engine ? null : engine))
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border-default bg-bg-surface">
      {/* header bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-default px-5 py-3.5">
        <div className="flex items-center gap-2 rounded-md border border-border-default bg-bg-subtle px-3 py-1.5 text-body-sm text-text-secondary">
          <GitBranch className="h-3.5 w-3.5" aria-hidden="true" />
          <span className="font-medium text-text-primary">Security Pipeline</span>
        </div>
        <StatusBadge status={status} />
      </div>

      {/* Real, live overall progress — the real fraction of this scan's jobs
          that have reached a terminal state (orchestrator's own GetProgress),
          not a fabricated animation. Shown for every scan, not just pentest,
          from the moment it starts until it's fully done. */}
      {scanStarted && (
        <div className="border-b border-border-default px-5 py-3">
          <ScanProgressBar pct={progress?.progress_pct ?? 0} />
        </div>
      )}

      {/* graph — left-to-right, GitHub Actions' own flow direction
          (documentation/09-ui-ux-design-system.md §4.8): one stage after
          another rather than a tall stacked column. The six parallel
          engines live inside one bordered group (same idea as GH Actions
          stacking parallel jobs inside one box with a single line in and
          a single line out) instead of each getting its own connector,
          which is what let the previous vertical version balloon in
          height for no benefit. */}
      <div className="overflow-x-auto p-6">
        <div className="flex min-w-max flex-col gap-4">
          <div className="flex items-center">
            <Node
              label="Scan start"
              icon={ShieldAlert}
              status={scanStarted ? 'succeeded' : 'not_run'}
            />

            <HConnector />

            <Node label="Workspace prep" icon={Package} status={workspaceStatus} />

            <HConnector />

            <div className="rounded-xl border border-border-default bg-bg-subtle/60 p-3">
              <div className="grid grid-cols-2 gap-3">
                {PARALLEL_ENGINES.map((engine) => {
                  const meta = ENGINE_META[engine]
                  const state = resolveEngineState(engine, progress, jobs)
                  const runnable = project ? isEngineRunnable(engine, project) : true
                  const clickable = scanStarted && runnable
                  const requirementReason = project
                    ? engineRequirementReason(engine, project)
                    : null
                  return (
                    <Node
                      key={engine}
                      label={meta.label}
                      icon={meta.icon}
                      status={state.status}
                      osvMark={meta.hasOsvMark}
                      sonarQubeMark={meta.hasSonarQubeMark}
                      kubernetesMark={meta.hasKubernetesMark}
                      githubMark={meta.hasGitHubMark}
                      geminiMark={meta.hasGeminiMark}
                      tooltip={state.reason ?? requirementReason}
                      onClick={clickable ? () => toggleEngine(engine) : undefined}
                      selected={selectedEngine === engine}
                    />
                  )
                })}
              </div>
            </div>

            <HConnector />

            <Node
              label="AI enrichment"
              icon={FileText}
              status="not_run"
              tooltip="Lands in Phase 10/11"
            />

            <HConnector />

            <Node label="Scoring" icon={ShieldAlert} status="not_run" tooltip="Lands in Phase 13" />
          </div>

          {/* Pentest branches independently off scan start — it needs no
              workspace, so it isn't part of the main left-to-right chain
              above; shown as its own row rather than forced to share a
              connector with a chain it doesn't depend on. */}
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2 text-caption text-text-tertiary">
              <div className="h-px w-6 border-t border-dashed border-border-strong" />
              <span>Runs independently — needs no workspace</span>
            </div>
            <Node
              label="Pentest"
              icon={ShieldAlert}
              status={pentestState.status}
              tooltip={
                pentestState.reason ??
                (project ? engineRequirementReason('pentest', project) : null)
              }
              onClick={
                scanStarted && (project ? isEngineRunnable('pentest', project) : true)
                  ? () => toggleEngine('pentest')
                  : undefined
              }
              selected={selectedEngine === 'pentest'}
            />
          </div>
        </div>
      </div>

      {selectedEngine && (
        <div className="px-6 pb-6">
          <EngineRunDetail
            engine={selectedEngine}
            job={jobs.find((j) => j.engine === selectedEngine)}
            liveProgress={progress?.engines.find((e) => e.engine === selectedEngine)}
            onClose={() => setSelectedEngine(null)}
          />
        </div>
      )}

      {/* footer: legend + real scan info, no fabricated fields */}
      <div className="flex flex-wrap items-start justify-between gap-6 border-t border-border-default bg-bg-subtle px-5 py-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
            Legend
          </span>
          <div className="flex flex-wrap gap-x-4 gap-y-1.5">
            {LEGEND.map(({ status: s, label }) => {
              const Icon = STATUS_ICON[s]
              return (
                <span
                  key={s}
                  className="flex items-center gap-1.5 text-caption text-text-secondary"
                >
                  <Icon className="h-3 w-3" style={{ color: STATUS_COLOR[s] }} aria-hidden="true" />
                  {label}
                </span>
              )
            })}
          </div>
        </div>

        {scan && (
          <div className="flex flex-col gap-1 text-caption">
            <span className="font-semibold uppercase tracking-wide text-text-tertiary">Scan</span>
            <div className="grid grid-cols-[auto_auto] gap-x-3 gap-y-0.5 text-text-secondary">
              <span>ID</span>
              <span className="font-mono text-text-primary">{scan.id.slice(0, 8)}</span>
              <span>Type</span>
              <span className="text-text-primary">{scan.type.replace(/_/g, ' ')}</span>
              {scan.branch && (
                <>
                  <span>Branch</span>
                  <span className="text-text-primary">{scan.branch}</span>
                </>
              )}
              <span>Queued</span>
              <span className="text-text-primary">{new Date(scan.queued_at).toLocaleString()}</span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
