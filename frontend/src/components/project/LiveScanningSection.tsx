import { useEffect, useState } from 'react'
import { AlertTriangle, Coins, GitBranch, Lock, RefreshCw, ShieldOff, Zap } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { Input } from '../ui/Input'
import { cn } from '../../lib/cn'
import { ApiError } from '../../lib/apiClient'
import { ENGINE_META } from '../../lib/engines'
import { formatDate } from '../../lib/format'
import type { Engine } from '../../lib/rulesApi'
import {
  disableLiveScan,
  enableLiveScan,
  getLiveScan,
  type LiveScanSettings,
} from '../../lib/scansApi'
import { useAuthStore } from '../../stores/authStore'
import { useBillingStore } from '../../stores/billingStore'
import { billingErrorMessage, estimateScan, formatTokens } from '../../lib/billingApi'

/** Plain-language labels for `last_delivery_status` (internal/modules/livescan). */
const DELIVERY_STATUS: Record<string, { label: string; tone: 'ok' | 'warn' | 'bad' }> = {
  ping_ok: { label: 'Connected — GitHub’s test delivery arrived', tone: 'ok' },
  accepted: { label: 'Event received', tone: 'ok' },
  scan_triggered: { label: 'Scan started', tone: 'ok' },
  ignored_event: { label: 'Event ignored (not a push or pull request)', tone: 'ok' },
  rate_limited: { label: 'Skipped — hourly automatic-scan limit reached', tone: 'warn' },
  scan_failed: { label: 'Couldn’t start the scan', tone: 'bad' },
  skipped_repository_mismatch: {
    label: 'Skipped — event came from a different repository than this project’s',
    tone: 'warn',
  },
  skipped_fork_pull_request: { label: 'Skipped — pull request from a fork', tone: 'warn' },
  skipped_no_repository: { label: 'Skipped — no repository attached', tone: 'bad' },
  skipped_no_allowed_engines: { label: 'Skipped — no scans selected', tone: 'bad' },
  insufficient_tokens: {
    label: 'Skipped — waiting for tokens (below your live-scan limit)',
    tone: 'warn',
  },
  plan_required: { label: 'Skipped — live scanning needs the Pro plan', tone: 'bad' },
  duplicate_commit: { label: 'Skipped — this commit was already scanned (no charge)', tone: 'ok' },
}

const FLOOR_OPTIONS = [0, 5, 10, 20, 30, 50]

/**
 * GitHub live scanning on ProjectSettingsPage (BUILD_GUIDE.md Phase 17
 * Part B): pushes and pull requests on the watched branches start the scans
 * picked here, automatically. Penetration tests are never offered — the
 * server refuses them too. Turning it on (or changing it) needs the
 * confirmation checkbox: every automatic scan then runs under the name of
 * whoever ticked it, which is what the exported report's authorisation
 * section shows.
 */
export function LiveScanningSection({
  projectId,
  hasRepository,
  defaultBranch,
}: {
  projectId: string
  hasRepository: boolean
  defaultBranch: string | null
}) {
  const role = useAuthStore((s) => s.user?.role)
  const userName = useAuthStore((s) => s.user?.displayName || s.user?.email || 'you')
  const isAdmin = role === 'admin'

  const [settings, setSettings] = useState<LiveScanSettings | null>(null)
  const [engines, setEngines] = useState<Set<Engine>>(new Set())
  const [branches, setBranches] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [floor, setFloor] = useState(10)
  const [pushesPerDay, setPushesPerDay] = useState(2)
  const [estimated, setEstimated] = useState<{ key: string; tokens: number } | null>(null)
  const billing = useBillingStore((s) => s.summary)
  const planLocked = !!billing && !billing.plan.live_scanning

  // Cost of one live scan with the engines picked, at the live-scan price.
  const engineKey = Array.from(engines).sort().join(',')
  useEffect(() => {
    if (!engineKey) return
    estimateScan({ engines: engineKey.split(',') as Engine[], trigger: 'webhook_push' })
      .then((e) => setEstimated({ key: engineKey, tokens: e.total }))
      .catch(() => setEstimated(null))
  }, [engineKey])
  const perPush = estimated && estimated.key === engineKey ? estimated.tokens : null

  function load() {
    getLiveScan(projectId)
      .then((s) => {
        setSettings(s)
        setEngines(new Set(s.enabled ? s.engines : s.allowed_engines))
        setBranches(s.enabled ? s.watched_branches.join(', ') : (defaultBranch ?? ''))
        setConfirmed(false)
        setFloor(s.enabled ? (s.min_balance_percent ?? 10) : 10)
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load live scanning.')
      })
  }

  useEffect(load, [projectId, defaultBranch])

  function toggleEngine(engine: Engine) {
    setEngines((prev) => {
      const next = new Set(prev)
      if (next.has(engine)) next.delete(engine)
      else next.add(engine)
      return next
    })
  }

  async function handleSave() {
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const updated = await enableLiveScan(projectId, {
        engines: Array.from(engines),
        watched_branches: branches
          .split(',')
          .map((b) => b.trim())
          .filter(Boolean),
        confirmed,
        min_balance_percent: floor,
      })
      setSettings(updated)
      setConfirmed(false)
      setNotice(
        settings?.enabled
          ? 'Live scanning settings saved.'
          : 'Live scanning is on. GitHub sends a test delivery right away — its status appears below.',
      )
    } catch (err) {
      setError(
        billingErrorMessage(err) ??
          (err instanceof ApiError ? err.problem.detail : 'Could not save live scanning.'),
      )
    } finally {
      setBusy(false)
    }
  }

  async function handleDisable() {
    if (!window.confirm('Turn off live scanning? Pushes will no longer start scans.')) return
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const res = await disableLiveScan(projectId)
      setNotice(
        res.github_hook_removed
          ? 'Live scanning is off and the webhook was removed from GitHub.'
          : 'Live scanning is off, but GitHub refused to remove the webhook (the token may have been revoked). Remove it under the repository’s Settings → Webhooks.',
      )
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not turn off live scanning.')
    } finally {
      setBusy(false)
    }
  }

  const status = settings?.last_delivery_status
    ? (DELIVERY_STATUS[settings.last_delivery_status] ?? {
        label: settings.last_delivery_status,
        tone: 'ok' as const,
      })
    : null

  return (
    <Card className={cn('mb-4', settings?.paused_reason && 'border-warning/40')}>
      <CardTitle className="flex items-center gap-2 text-h3">
        <Zap className="h-4 w-4" aria-hidden="true" />
        Live scanning (GitHub)
        {settings?.enabled && (
          <span
            className={cn(
              'ml-1 inline-flex items-center rounded-full px-2 py-0.5 text-caption font-semibold',
              settings.paused_reason ? 'bg-warning/10 text-warning' : 'bg-success/10 text-success',
            )}
          >
            {settings.paused_reason ? 'Paused' : 'On'}
          </span>
        )}
      </CardTitle>
      <CardDescription className="mt-1">
        Scan automatically whenever someone pushes to a watched branch or opens or updates a pull
        request into one.
      </CardDescription>

      {error && (
        <p role="alert" className="mt-3 text-body-sm text-danger">
          {error}
        </p>
      )}
      {notice && <p className="mt-3 text-body-sm text-text-secondary">{notice}</p>}

      {!hasRepository ? (
        <p className="mt-4 text-body-sm text-text-secondary">
          Attach a GitHub repository above to use live scanning.
        </p>
      ) : settings === null ? null : (
        <>
          {settings.paused_reason && (
            <div
              role="status"
              className="mt-4 flex gap-2 rounded-lg border border-warning/40 bg-warning/5 p-3 text-body-sm text-text-primary"
            >
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
              <span>
                Live scanning paused itself {settings.paused_at && formatDate(settings.paused_at)}{' '}
                because pushes were starting scans unusually fast ({settings.paused_reason}). Check
                what&rsquo;s pushing to this repository, then confirm below to resume.
              </span>
            </div>
          )}

          {settings.enabled && settings.last_delivery_status === 'insufficient_tokens' && (
            <div
              role="status"
              className="mt-4 flex items-center gap-2 rounded-lg border border-warning/40 bg-warning/5 p-3 text-body-sm text-text-primary"
            >
              <Coins className="h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
              <span className="flex-1">
                Live scans are waiting for tokens. They start again by themselves once you have
                more.
              </span>
              <Link
                to="/pricing"
                className="rounded-md bg-accent px-2.5 py-1 text-caption font-semibold text-text-inverse hover:opacity-90"
              >
                Buy tokens
              </Link>
            </div>
          )}

          {planLocked && (
            <div className="mt-4 flex items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 p-3 text-body-sm text-text-primary">
              <Lock className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
              <span className="flex-1">Live scanning is a Pro feature.</span>
              <Link
                to="/pricing"
                className="rounded-md bg-accent px-2.5 py-1 text-caption font-semibold text-text-inverse hover:opacity-90"
              >
                Upgrade to Pro
              </Link>
            </div>
          )}

          {settings.enabled && (
            <dl className="mt-4 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-body-sm">
              <dt className="text-text-tertiary">Authorised by</dt>
              <dd className="text-text-primary">
                {settings.enabled_by_name ?? 'a former member'}
                {settings.attested_at && ` on ${formatDate(settings.attested_at)}`}
              </dd>
              <dt className="text-text-tertiary">Last delivery</dt>
              <dd className="flex items-center gap-2">
                {settings.last_delivery_at ? (
                  <span
                    className={cn(
                      status?.tone === 'bad' && 'text-danger',
                      status?.tone === 'warn' && 'text-warning',
                      status?.tone === 'ok' && 'text-text-primary',
                    )}
                  >
                    {status?.label} · {formatDate(settings.last_delivery_at)}
                  </span>
                ) : (
                  <span className="text-text-secondary">
                    Nothing received yet. If a push doesn&rsquo;t show up here, check the
                    webhook&rsquo;s Recent Deliveries on GitHub.
                  </span>
                )}
                <button
                  type="button"
                  onClick={load}
                  aria-label="Refresh delivery status"
                  className="text-text-tertiary hover:text-text-primary"
                >
                  <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
                </button>
              </dd>
            </dl>
          )}

          <fieldset className="mt-4" disabled={!isAdmin || busy}>
            <legend className="mb-2 text-body-sm font-medium text-text-primary">
              Scans to run on each push
            </legend>
            <div className="flex flex-wrap gap-2">
              {settings.allowed_engines.map((engine) => {
                const selected = engines.has(engine)
                return (
                  <button
                    key={engine}
                    type="button"
                    aria-pressed={selected}
                    onClick={() => toggleEngine(engine)}
                    className={cn(
                      'rounded-full border px-3 py-1 text-caption font-medium transition-colors disabled:opacity-60',
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
            <p className="mt-2 flex items-center gap-1.5 text-caption text-text-tertiary">
              <ShieldOff className="h-3.5 w-3.5" aria-hidden="true" />
              Penetration tests never run automatically — start those by hand.
            </p>

            <label
              htmlFor="livescan-branches"
              className="mt-4 mb-1 flex items-center gap-1.5 text-body-sm text-text-secondary"
            >
              <GitBranch className="h-3.5 w-3.5" aria-hidden="true" />
              Watched branches (comma-separated)
            </label>
            <Input
              id="livescan-branches"
              value={branches}
              placeholder={defaultBranch ?? 'main'}
              onChange={(e) => setBranches(e.target.value)}
            />

            {!settings.enabled && (
              <p className="mt-3 text-caption text-text-tertiary">
                GuardPipe adds a webhook to the repository using this project&rsquo;s GitHub token,
                so the token needs permission to manage webhooks: the <code>admin:repo_hook</code>{' '}
                scope for a classic token, or &ldquo;Webhooks: read and write&rdquo; for a
                fine-grained one.
              </p>
            )}

            {billing && perPush !== null && (
              <div className="mt-4 rounded-lg border border-border-default bg-bg-subtle/60 p-3 text-body-sm">
                <p className="flex flex-wrap items-center gap-1.5 text-text-primary">
                  <Coins className="h-4 w-4 text-accent" aria-hidden="true" />
                  <strong className="tabular-nums">~{formatTokens(perPush)}</strong> tokens per push
                  (live price). At
                  <select
                    aria-label="Pushes per day"
                    value={pushesPerDay}
                    onChange={(e) => setPushesPerDay(Number(e.target.value))}
                    className="rounded border border-border-default bg-bg-surface px-1 py-0.5"
                  >
                    {[1, 2, 5, 10, 20].map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                  </select>
                  pushes a day that is
                  <strong
                    className={cn(
                      'tabular-nums',
                      perPush * pushesPerDay * 30 + billing.used_this_cycle > billing.monthly_grant
                        ? 'text-warning'
                        : 'text-text-primary',
                    )}
                  >
                    ~{formatTokens(perPush * pushesPerDay * 30)}/month
                  </strong>
                  <span className="text-text-secondary">
                    (
                    {Math.round(
                      ((perPush * pushesPerDay * 30) / Math.max(billing.monthly_grant, 1)) * 100,
                    )}
                    % of your plan)
                  </span>
                </p>
                <label className="mt-2 flex flex-wrap items-center gap-1.5 text-text-secondary">
                  Pause live scans when fewer than
                  <select
                    value={floor}
                    onChange={(e) => setFloor(Number(e.target.value))}
                    className="rounded border border-border-default bg-bg-surface px-1 py-0.5 text-text-primary"
                  >
                    {FLOOR_OPTIONS.map((p) => (
                      <option key={p} value={p}>
                        {formatTokens((billing.monthly_grant * p) / 100)} ({p}%)
                      </option>
                    ))}
                  </select>
                  tokens remain, so manual scans always have tokens left.
                </label>
              </div>
            )}

            <label className="mt-4 flex items-start gap-2 rounded-lg border border-border-default p-3 text-body-sm text-text-primary">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
                className="mt-0.5 h-4 w-4 shrink-0 rounded border-border-strong accent-accent"
              />
              <span>
                I authorise GuardPipe to scan this repository automatically whenever the watched
                branches change. Automatic scans and their reports are recorded under my name (
                {userName}).
              </span>
            </label>

            <div className="mt-4 flex flex-wrap gap-2">
              <Button
                loading={busy}
                disabled={!confirmed || engines.size === 0 || planLocked}
                onClick={() => void handleSave()}
              >
                {settings.enabled
                  ? settings.paused_reason
                    ? 'Resume live scanning'
                    : 'Save changes'
                  : 'Turn on live scanning'}
              </Button>
              {settings.enabled && (
                <Button variant="secondary" loading={busy} onClick={() => void handleDisable()}>
                  Turn off
                </Button>
              )}
            </div>
          </fieldset>
          {!isAdmin && (
            <p className="mt-2 text-caption text-text-tertiary">
              Only organisation admins can change live scanning.
            </p>
          )}
        </>
      )}
    </Card>
  )
}
