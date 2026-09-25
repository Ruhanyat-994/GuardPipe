import { useEffect, useState } from 'react'
import { Calendar, Trash2 } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { cn } from '../../lib/cn'
import { ApiError } from '../../lib/apiClient'
import { ALL_ENGINES, ENGINE_META, isEngineEnabled } from '../../lib/engines'
import type { Engine } from '../../lib/rulesApi'
import { listMembers, type MemberSummary } from '../../lib/organizationApi'
import {
  createSchedule,
  deleteSchedule,
  listSchedules,
  updateSchedule,
  type ScanSchedule,
} from '../../lib/scansApi'
import { useAuthStore } from '../../stores/authStore'
import { formatDate } from '../../lib/format'

type Preset = 'daily' | 'weekly' | 'custom'

const PRESET_CRON: Record<Exclude<Preset, 'custom'>, string> = {
  daily: '0 9 * * *',
  weekly: '0 9 * * 1',
}

/** Plain words for last_run_status. */
const SCHEDULE_STATUS: Record<string, string> = {
  triggered: 'started',
  failed: 'failed',
  skipped_insufficient_tokens: 'skipped — not enough tokens',
  skipped_plan_required: 'skipped — needs the Pro plan',
}

/**
 * Scheduled Scans section on ProjectSettingsPage (BUILD_GUIDE.md Phase 15)
 * — a cron builder in the same plain-language-plus-technical shape Phase
 * 12's scan-intensity dials already use (Daily/Weekly/Custom presets, the
 * underlying cron expression always visible and editable), an assignee
 * picker (org members only), and a plain engine checklist reusing
 * lib/engines.ts's own catalogue rather than duplicating it — a simpler
 * picker than ScanLauncher's (no live attestation flow inline; pentest can
 * be scheduled once a target is already attested via the Targets tab, the
 * same structural requirement CreateScan itself enforces server-side).
 */
export function ScheduledScansSection({ projectId }: { projectId: string }) {
  const orgId = useAuthStore((s) => s.user?.orgId ?? '')
  const [schedules, setSchedules] = useState<ScanSchedule[] | null>(null)
  const [members, setMembers] = useState<MemberSummary[]>([])
  const [error, setError] = useState<string | null>(null)

  const [preset, setPreset] = useState<Preset>('weekly')
  const [cron, setCron] = useState(PRESET_CRON.weekly)
  const [runAll, setRunAll] = useState(true)
  const [engines, setEngines] = useState<Set<Engine>>(new Set())
  const [assignee, setAssignee] = useState('')
  const [creating, setCreating] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)

  function refresh() {
    listSchedules(projectId)
      .then((res) => setSchedules(res.data))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load scheduled scans.')
      })
  }

  useEffect(refresh, [projectId])
  useEffect(() => {
    if (!orgId) return
    listMembers(orgId)
      .then((res) => setMembers(res.data))
      .catch(() => setMembers([]))
  }, [orgId])

  function handlePresetChange(p: Preset) {
    setPreset(p)
    if (p !== 'custom') setCron(PRESET_CRON[p])
  }

  async function handleCreate() {
    setCreating(true)
    setError(null)
    try {
      await createSchedule(projectId, {
        cron_expression: cron,
        profile: {
          type: runAll ? 'full_supply_chain' : 'partial',
          ...(runAll ? {} : { engines: Array.from(engines) }),
        },
        ...(assignee ? { assigned_to: assignee } : {}),
      })
      setEngines(new Set())
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not create the schedule.')
    } finally {
      setCreating(false)
    }
  }

  async function handleToggle(schedule: ScanSchedule) {
    setBusyId(schedule.id)
    try {
      await updateSchedule(schedule.id, { enabled: !schedule.enabled })
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not update the schedule.')
    } finally {
      setBusyId(null)
    }
  }

  async function handleDelete(scheduleId: string) {
    if (!window.confirm('Delete this scheduled scan?')) return
    setBusyId(scheduleId)
    try {
      await deleteSchedule(scheduleId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not delete the schedule.')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <Card className="mb-4">
      <CardTitle className="flex items-center gap-2 text-h3">
        <Calendar className="h-4 w-4" aria-hidden="true" />
        Scheduled scans
      </CardTitle>
      <CardDescription className="mt-1">
        Run this project&rsquo;s scans automatically on a recurring cadence — no more than once an
        hour.
      </CardDescription>

      {error && (
        <p role="alert" className="mt-3 text-body-sm text-danger">
          {error}
        </p>
      )}

      {schedules && schedules.length > 0 && (
        <ul className="mt-4 flex flex-col divide-y divide-border-default">
          {schedules.map((s) => (
            <li key={s.id} className="flex flex-wrap items-center gap-3 py-3">
              <span
                className={cn(
                  'inline-flex items-center rounded-full px-2 py-0.5 text-caption font-semibold',
                  s.enabled ? 'bg-success/10 text-success' : 'bg-bg-subtle text-text-tertiary',
                )}
              >
                {s.enabled ? 'Enabled' : 'Disabled'}
              </span>
              <code className="text-caption text-text-secondary">{s.cron_expression}</code>
              <span className="text-caption text-text-tertiary">
                {s.profile.type === 'full_supply_chain'
                  ? 'All engines'
                  : s.profile.engines?.join(', ')}
              </span>
              <span className="ml-auto text-caption text-text-tertiary">
                Next: {formatDate(s.next_run_at)}
                {s.last_run_status && (
                  <>
                    {' '}
                    · Last:{' '}
                    <span
                      className={
                        s.last_run_status.startsWith('skipped') ? 'text-warning' : undefined
                      }
                    >
                      {SCHEDULE_STATUS[s.last_run_status] ?? s.last_run_status}
                    </span>
                  </>
                )}
              </span>
              <Button
                variant="secondary"
                size="sm"
                loading={busyId === s.id}
                onClick={() => void handleToggle(s)}
              >
                {s.enabled ? 'Disable' : 'Enable'}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                loading={busyId === s.id}
                onClick={() => void handleDelete(s.id)}
              >
                <Trash2 className="h-3.5 w-3.5 text-danger" aria-hidden="true" />
              </Button>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-4 rounded-lg border border-border-default p-4">
        <p className="mb-2 text-body-sm font-medium text-text-primary">New schedule</p>
        <div className="flex flex-wrap items-center gap-2">
          {(['daily', 'weekly', 'custom'] as Preset[]).map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => handlePresetChange(p)}
              className={cn(
                'rounded-full border px-3 py-1 text-caption font-medium capitalize transition-colors',
                preset === p
                  ? 'border-accent bg-accent/10 text-accent'
                  : 'border-border-default text-text-secondary hover:border-border-strong',
              )}
            >
              {p}
            </button>
          ))}
          <code className="ml-2 rounded bg-bg-subtle px-2 py-1 text-caption text-text-secondary">
            {preset === 'custom' ? (
              <input
                value={cron}
                onChange={(e) => setCron(e.target.value)}
                className="w-40 bg-transparent outline-none"
                placeholder="0 9 * * 1"
              />
            ) : (
              cron
            )}
          </code>
        </div>

        <label className="mt-3 flex items-center gap-2 text-body-sm text-text-primary">
          <input
            type="checkbox"
            checked={runAll}
            onChange={(e) => setRunAll(e.target.checked)}
            className="h-4 w-4 rounded border-border-strong accent-accent"
          />
          Run every available engine
        </label>
        {!runAll && (
          <div className="mt-2 flex flex-wrap gap-2">
            {ALL_ENGINES.filter(isEngineEnabled).map((engine) => {
              const selected = engines.has(engine)
              return (
                <button
                  key={engine}
                  type="button"
                  aria-pressed={selected}
                  onClick={() =>
                    setEngines((prev) => {
                      const next = new Set(prev)
                      if (next.has(engine)) next.delete(engine)
                      else next.add(engine)
                      return next
                    })
                  }
                  className={cn(
                    'rounded-full border px-3 py-1 text-caption font-medium transition-colors',
                    selected
                      ? 'border-accent bg-accent/10 text-accent'
                      : 'border-border-default text-text-secondary hover:border-border-strong',
                  )}
                >
                  {ENGINE_META[engine].label}
                </button>
              )
            })}
          </div>
        )}

        <div className="mt-3">
          <label
            htmlFor="schedule-assignee"
            className="mb-1 block text-body-sm text-text-secondary"
          >
            Assign to (optional)
          </label>
          <select
            id="schedule-assignee"
            value={assignee}
            onChange={(e) => setAssignee(e.target.value)}
            className="h-9 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary"
          >
            <option value="">Unassigned</option>
            {members.map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.display_name}
              </option>
            ))}
          </select>
        </div>

        <Button
          className="mt-4"
          loading={creating}
          disabled={!runAll && engines.size === 0}
          onClick={() => void handleCreate()}
        >
          Create schedule
        </Button>
      </div>
    </Card>
  )
}
