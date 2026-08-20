import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  Circle,
  Clock,
  Inbox,
  Loader2,
  MinusCircle,
  X,
  XCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { ENGINE_META } from '../../lib/engines'
import { humanizeErrorReason } from '../../lib/engineRunMessages'
import type { Engine } from '../../lib/rulesApi'
import type { Job } from '../../lib/scansApi'
import { EmptyState } from '../ui/EmptyState'
import { GeminiMark } from '../icons/GeminiMark'
import { GitHubMark } from '../icons/GitHubMark'
import { KubernetesMark } from '../icons/KubernetesMark'
import { OsvMark } from '../icons/OsvMark'
import { SonarQubeMark } from '../icons/SonarQubeMark'

type StepState = 'pending' | 'active' | 'done' | 'skipped' | 'failed'

const STEP_ICON: Record<StepState, LucideIcon> = {
  pending: Circle,
  active: Loader2,
  done: CheckCircle2,
  skipped: MinusCircle,
  failed: XCircle,
}

const STEP_COLOR: Record<StepState, string> = {
  pending: 'var(--text-tertiary)',
  active: 'var(--accent)',
  done: 'var(--success)',
  skipped: 'var(--text-tertiary)',
  failed: 'var(--danger)',
}

interface Step {
  label: string
  state: StepState
  meta?: string
}

function formatDuration(startedAt: string | null, finishedAt: string | null): string | null {
  if (!startedAt) return null
  const start = new Date(startedAt).getTime()
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const ms = Math.max(0, end - start)
  if (ms < 1000) return '<1s'
  const totalSeconds = Math.round(ms / 1000)
  if (totalSeconds < 60) return `${totalSeconds}s`
  return `${Math.floor(totalSeconds / 60)}m ${totalSeconds % 60}s`
}

function statNumber(stats: Record<string, unknown> | null | undefined, key: string): number | null {
  const v = stats?.[key]
  return typeof v === 'number' ? v : null
}

/**
 * The three real lifecycle stages every job actually goes through
 * (queued -> running -> persisted, worker.go) — never more than that, since
 * that's all the backend genuinely tracks per job. The middle step's label
 * is the engine's own generic `activity` phrase, which is what keeps this
 * honest about not narrating rule-level internals while still visibly
 * "doing something" (the interactivity this panel exists for).
 */
function deriveSteps(job: Job | undefined, activityLabel: string): Step[] {
  if (!job) {
    return [
      { label: 'Queued', state: 'pending' },
      { label: activityLabel, state: 'pending' },
      { label: 'Collect results', state: 'pending' },
    ]
  }
  const duration = formatDuration(job.started_at, job.finished_at) ?? undefined
  switch (job.status) {
    case 'queued':
      return [
        { label: 'Queued', state: 'done' },
        { label: activityLabel, state: 'pending' },
        { label: 'Collect results', state: 'pending' },
      ]
    case 'running':
      return [
        { label: 'Queued', state: 'done' },
        { label: activityLabel, state: 'active' },
        { label: 'Collect results', state: 'pending' },
      ]
    case 'succeeded':
      return [
        { label: 'Queued', state: 'done' },
        { label: activityLabel, state: 'done', meta: duration },
        { label: 'Collect results', state: 'done' },
      ]
    case 'failed': {
      const collected = job.finding_count > 0
      return [
        { label: 'Queued', state: 'done' },
        { label: activityLabel, state: 'failed', meta: duration },
        {
          label: collected ? 'Partial results collected' : 'Collect results',
          state: collected ? 'done' : 'skipped',
        },
      ]
    }
    case 'skipped':
      return [
        { label: 'Queued', state: 'done' },
        { label: activityLabel, state: 'skipped' },
        { label: 'Collect results', state: 'skipped' },
      ]
    case 'cancelled':
      return [
        { label: 'Queued', state: 'done' },
        { label: activityLabel, state: 'skipped' },
        { label: 'Collect results', state: 'skipped' },
      ]
  }
}

function StepRow({ step }: { step: Step }) {
  const Icon = STEP_ICON[step.state]
  const color = STEP_COLOR[step.state]
  return (
    <div className="flex items-center gap-2.5 py-1.5">
      <Icon
        className={step.state === 'active' ? 'h-4 w-4 shrink-0 animate-spin' : 'h-4 w-4 shrink-0'}
        style={{ color }}
        aria-hidden="true"
      />
      <span
        className="text-body-sm"
        style={{ color: step.state === 'pending' ? 'var(--text-tertiary)' : 'var(--text-primary)' }}
      >
        {step.label}
      </span>
      {step.meta && <span className="ml-auto text-caption text-text-tertiary">{step.meta}</span>}
    </div>
  )
}

/**
 * The panel a click on a SupplyChainPipeline node opens
 * (documentation/09-ui-ux-design-system.md §4.8) — a per-engine "what's it
 * doing" drill-down, GitHub Actions'-job-log in spirit, GuardPipe's own
 * content in substance: three honest lifecycle stages plus whatever generic,
 * non-sensitive stats the engine itself reported (files scanned, rules
 * evaluated, finding count), never the specific rules/patterns that fired.
 * A skipped or failed job gets a calm/explanatory state instead of steps
 * that pretend a run happened.
 */
export function EngineRunDetail({
  engine,
  job,
  onClose,
}: {
  engine: Engine
  job: Job | undefined
  onClose: () => void
}) {
  const meta = ENGINE_META[engine]
  const Icon = meta.icon
  const steps = deriveSteps(job, meta.activity)
  const rulesEvaluated = statNumber(job?.stats, 'rules_evaluated')
  const filesScanned = statNumber(job?.stats, 'files_scanned')
  const duration = job ? formatDuration(job.started_at, job.finished_at) : null

  return (
    <div className="animate-reveal rounded-lg border border-border-default bg-bg-subtle">
      <div className="flex items-center gap-3 border-b border-border-default px-5 py-3.5">
        <div
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full"
          style={{
            backgroundColor: 'color-mix(in srgb, var(--accent) 15%, transparent)',
            color: 'var(--accent)',
          }}
        >
          <Icon className="h-4 w-4" aria-hidden="true" />
        </div>
        <div className="flex items-center gap-1.5">
          <span className="font-semibold text-text-primary">{meta.label}</span>
          {meta.hasOsvMark && <OsvMark />}
          {meta.hasSonarQubeMark && <SonarQubeMark />}
          {meta.hasKubernetesMark && <KubernetesMark />}
          {meta.hasGitHubMark && <GitHubMark className="h-3 w-3" />}
          {meta.hasGeminiMark && <GeminiMark />}
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          className="ml-auto rounded-full p-1 text-text-tertiary hover:bg-bg-surface hover:text-text-primary"
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      <div className="px-5 py-4">
        <div className="flex flex-col divide-y divide-border-default/60">
          {steps.map((step) => (
            <StepRow key={step.label} step={step} />
          ))}
        </div>

        {job?.status === 'succeeded' && (
          <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 border-t border-border-default pt-3 text-caption text-text-secondary">
            {filesScanned !== null && (
              <span>
                <span className="font-semibold text-text-primary">{filesScanned}</span> files
                scanned
              </span>
            )}
            {rulesEvaluated !== null && (
              <span>
                <span className="font-semibold text-text-primary">{rulesEvaluated}</span> rules
                evaluated
              </span>
            )}
            <span>
              <span className="font-semibold text-text-primary">{job.finding_count}</span> finding
              {job.finding_count === 1 ? '' : 's'}
            </span>
            {duration && <span className="ml-auto">Took {duration}</span>}
          </div>
        )}

        {job?.status === 'skipped' && (
          <EmptyState
            icon={Inbox}
            title={`Nothing for ${meta.label} to check`}
            description={
              job.skip_reason ?? 'This project has nothing matching what this check looks for.'
            }
            tone="neutral"
            size="compact"
            className="mt-1"
          />
        )}

        {job?.status === 'failed' && (
          <>
            <EmptyState
              icon={AlertTriangle}
              title="This check didn't finish"
              description={humanizeErrorReason(job.error_reason)}
              tone="danger"
              size="compact"
              className="mt-1"
            />
            {job.finding_count > 0 && (
              <p className="text-center text-caption text-text-tertiary">
                {job.finding_count} finding{job.finding_count === 1 ? '' : 's'} collected before the
                failure {job.finding_count === 1 ? 'is' : 'are'} still included below.
              </p>
            )}
          </>
        )}

        {job?.status === 'cancelled' && (
          <EmptyState
            icon={Ban}
            title="Cancelled"
            description="This scan was cancelled before this check finished."
            tone="neutral"
            size="compact"
            className="mt-1"
          />
        )}

        {!job && (
          <EmptyState
            icon={Clock}
            title="Waiting to start"
            description="This check hasn't been picked up by a worker yet."
            tone="neutral"
            size="compact"
            className="mt-1"
          />
        )}
      </div>
    </div>
  )
}
