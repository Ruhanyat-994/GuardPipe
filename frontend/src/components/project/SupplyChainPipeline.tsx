import {
  Boxes,
  Code2,
  Container as ContainerIcon,
  FileText,
  Package,
  ShieldAlert,
  Workflow,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '../../lib/cn'
import type { Engine } from '../../lib/rulesApi'
import type { Job, JobStatus, Progress } from '../../lib/scansApi'
import { OsvMark } from '../icons/OsvMark'

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
 * (GlobalSearch/NotificationPanel, documentation/09 §4.4).
 */

type NodeStatus = 'not_run' | 'running' | 'succeeded' | 'failed' | 'skipped'

const STATUS_COLOR: Record<NodeStatus, string> = {
  not_run: 'var(--text-tertiary)',
  running: 'var(--accent)',
  succeeded: 'var(--success)',
  failed: 'var(--danger)',
  skipped: 'var(--text-tertiary)',
}

const ENGINE_META: Record<Engine, { label: string; icon: LucideIcon; hasOsvMark?: boolean }> = {
  docreview: { label: 'Docs', icon: FileText },
  codescan: { label: 'Code', icon: Code2 },
  depscan: { label: 'Deps', icon: Package, hasOsvMark: true },
  containerscan: { label: 'Containers', icon: ContainerIcon },
  k8sscan: { label: 'K8s', icon: Boxes },
  cicdscan: { label: 'CI/CD', icon: Workflow },
  pentest: { label: 'Pentest', icon: ShieldAlert },
}

const PARALLEL_ENGINES: Engine[] = ['docreview', 'codescan', 'depscan', 'containerscan', 'k8sscan', 'cicdscan']

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

function resolveEngineState(engine: Engine, progress: Progress | null, jobs: Job[]): EngineNodeState {
  const live = progress?.engines.find((e) => e.engine === engine)
  if (live) {
    return { status: jobStatusToNodeStatus(live.status), findingCount: live.finding_count, reason: null }
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

function Node({
  label,
  icon: Icon,
  status,
  osvMark,
  tooltip,
}: {
  label: string
  icon: LucideIcon
  status: NodeStatus
  osvMark?: boolean
  tooltip?: string | null
}) {
  const color = STATUS_COLOR[status]
  return (
    <div
      className="flex min-w-[100px] flex-col items-center gap-1.5 rounded-lg border bg-bg-surface px-3 py-3 text-center shadow-sm"
      style={{ borderColor: status === 'not_run' ? undefined : `color-mix(in srgb, ${color} 40%, transparent)` }}
      title={tooltip ?? undefined}
    >
      <div
        className={cn('flex h-9 w-9 items-center justify-center rounded-full', status === 'running' && 'animate-pulse')}
        style={{ backgroundColor: `color-mix(in srgb, ${color} 15%, transparent)`, color }}
      >
        <Icon className="h-4.5 w-4.5" aria-hidden="true" />
      </div>
      <div className="flex items-center gap-1">
        <span className="text-caption font-medium text-text-primary">{label}</span>
        {osvMark && <OsvMark />}
      </div>
      <span className="text-caption capitalize" style={{ color }}>
        {status.replace('_', ' ')}
      </span>
    </div>
  )
}

function Connector({ vertical = false }: { vertical?: boolean }) {
  return (
    <div
      className={cn('border-border-default', vertical ? 'h-4 w-px border-l' : 'h-px flex-1 border-t')}
      aria-hidden="true"
    />
  )
}

export function SupplyChainPipeline({ progress, jobs }: { progress: Progress | null; jobs: Job[] }) {
  const scanStarted = progress !== null || jobs.length > 0
  const anyRunningOrDone = jobs.some((j) => j.status !== 'queued') || (progress?.progress_pct ?? 0) > 0
  const workspaceStatus: NodeStatus = !scanStarted ? 'not_run' : anyRunningOrDone ? 'succeeded' : 'running'

  const pentestState = resolveEngineState('pentest', progress, jobs)

  return (
    <div className="overflow-x-auto rounded-lg border border-border-default bg-bg-subtle p-6">
      <div className="flex min-w-max flex-col items-center gap-3">
        {/* scan start -> workspace prep / pentest branch */}
        <div className="flex items-center gap-2">
          <Node label="Scan start" icon={ShieldAlert} status={scanStarted ? 'succeeded' : 'not_run'} />
        </div>
        <div className="flex w-full items-start justify-center gap-8">
          <div className="flex flex-col items-center gap-3">
            <Connector vertical />
            <Node label="Workspace prep" icon={Package} status={workspaceStatus} />
            <Connector vertical />
            <div className="flex flex-wrap items-start justify-center gap-3">
              {PARALLEL_ENGINES.map((engine) => {
                const meta = ENGINE_META[engine]
                const state = resolveEngineState(engine, progress, jobs)
                return (
                  <Node
                    key={engine}
                    label={meta.label}
                    icon={meta.icon}
                    status={state.status}
                    osvMark={meta.hasOsvMark}
                    tooltip={state.reason}
                  />
                )
              })}
            </div>
          </div>

          <div className="flex flex-col items-center gap-3 pt-9">
            <span className="text-caption text-text-tertiary">needs no workspace</span>
            <Node label="Pentest" icon={ShieldAlert} status={pentestState.status} tooltip={pentestState.reason} />
          </div>
        </div>

        <Connector vertical />
        <Node label="AI enrichment" icon={FileText} status="not_run" tooltip="Lands in Phase 10/11" />
        <Connector vertical />
        <Node label="Scoring" icon={ShieldAlert} status="not_run" tooltip="Lands in Phase 13" />
      </div>
    </div>
  )
}
