import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { CheckCircle2, Coins, ExternalLink, X } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { ledgerLabel, meterTone, TONE_BAR } from '../lib/billingUi'
import { cn } from '../lib/cn'
import { ENGINE_META } from '../lib/engines'
import { formatDate } from '../lib/format'
import {
  billingErrorMessage,
  cancelSubscription,
  describeReason,
  formatPrice,
  formatShortDate,
  formatTokens,
  getLedger,
  resumeSubscription,
  TRIGGER_LABEL,
  type LedgerEntry,
} from '../lib/billingApi'
import { notifyBillingChanged } from '../lib/billingEvents'
import { useAuthStore } from '../stores/authStore'
import { useBillingStore } from '../stores/billingStore'
import { useCountUp } from '../hooks/useCountUp'

/**
 * Org Settings → Billing: the plan, the big token meter, where tokens went
 * this cycle, and the full ledger — every grant, charge, refund and expiry,
 * each with the balance after it.
 */
export function BillingPage() {
  const [params, setParams] = useSearchParams()
  const purchased = params.get('purchased')
  const user = useAuthStore((s) => s.user)
  const isAdmin = user?.role === 'admin'
  const summary = useBillingStore((s) => s.summary)
  const unavailable = useBillingStore((s) => s.unavailable)
  const refresh = useBillingStore((s) => s.refresh)

  const [ledger, setLedger] = useState<LedgerEntry[]>([])
  const [nextBefore, setNextBefore] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadLedger = useCallback(() => {
    getLedger(20)
      .then((p) => {
        setLedger(p.data)
        setNextBefore(p.next_before)
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])
  // Reload the history whenever the balance moves.
  useEffect(() => {
    loadLedger()
  }, [loadLedger, summary?.balance])

  const shown = useCountUp(summary?.balance ?? 0, 900)

  async function more() {
    if (!nextBefore) return
    setLoadingMore(true)
    try {
      const p = await getLedger(20, nextBefore)
      setLedger((l) => [...l, ...p.data])
      setNextBefore(p.next_before)
    } finally {
      setLoadingMore(false)
    }
  }

  async function toggleCancel(cancel: boolean) {
    if (
      cancel &&
      !window.confirm('Cancel Pro? It keeps working until the end of the current period.')
    )
      return
    setBusy(true)
    setError(null)
    try {
      await (cancel ? cancelSubscription() : resumeSubscription())
      notifyBillingChanged()
    } catch (err) {
      setError(billingErrorMessage(err) ?? 'Could not update the subscription.')
    } finally {
      setBusy(false)
    }
  }

  if (unavailable) {
    return (
      <main className="mx-auto max-w-4xl px-6 py-8">
        <h1 className="text-h1 text-text-primary">Billing</h1>
        <p className="mt-2 text-body text-text-secondary">
          Billing isn&rsquo;t available in this session.
        </p>
      </main>
    )
  }
  if (!summary) {
    return (
      <main className="mx-auto max-w-4xl px-6 py-8 text-body-sm text-text-tertiary">
        Loading billing…
      </main>
    )
  }

  const grant = Math.max(summary.monthly_grant, 1)
  const tone = meterTone(summary.balance, summary.monthly_grant)
  const planPct = Math.min(100, (summary.plan_grant_remaining / grant) * 100)
  const topupPct = Math.min(100 - planPct, (summary.topup_remaining / grant) * 100)
  const isPaid = summary.plan.code !== 'free'

  // Where tokens went this cycle.
  const byEngine = new Map<string, number>()
  const byTrigger = new Map<string, number>()
  for (const u of summary.usage) {
    byEngine.set(u.engine, (byEngine.get(u.engine) ?? 0) + u.tokens)
    const t = u.trigger_source.startsWith('webhook') ? 'live' : u.trigger_source
    byTrigger.set(t, (byTrigger.get(t) ?? 0) + u.tokens)
  }
  const engineRows = [...byEngine.entries()].filter(([, v]) => v > 0).sort((a, b) => b[1] - a[1])
  const maxEngine = Math.max(1, ...engineRows.map(([, v]) => v))

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <h1 className="mb-1 text-h1 text-text-primary">Billing</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        Your plan, your token balance, and where every token went.
        {summary.mode === 'demo' && ' Payments are in demo mode.'}
      </p>

      {purchased && (
        <div className="mb-6 flex items-center gap-3 rounded-lg border border-success/40 bg-success/10 px-4 py-3 text-body-sm text-text-primary">
          <CheckCircle2 className="h-5 w-5 shrink-0 text-success" aria-hidden="true" />
          <span className="flex-1">
            {purchased.startsWith('topup')
              ? 'Top-up added to your balance.'
              : `${summary.plan.name} is active — ${formatTokens(summary.monthly_grant)} tokens added.`}
          </span>
          <button
            type="button"
            aria-label="Dismiss"
            onClick={() => setParams({})}
            className="text-text-tertiary hover:text-text-primary"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}
      {error && (
        <p
          role="alert"
          className="mb-4 rounded-md border border-danger/30 bg-danger/5 px-4 py-2 text-body-sm text-danger"
        >
          {error}
        </p>
      )}

      <div className="grid grid-cols-1 gap-6 md:grid-cols-5">
        <Card className="md:col-span-3">
          <p className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
            Token balance
          </p>
          <p className="mt-1 flex items-baseline gap-2">
            <span className="text-display-section font-semibold tabular-nums text-text-primary">
              {formatTokens(shown)}
            </span>
            <span className="text-body text-text-secondary">tokens</span>
          </p>
          <div
            className="mt-4 flex h-3 overflow-hidden rounded-full bg-bg-subtle"
            role="img"
            aria-label={`${formatTokens(summary.plan_grant_remaining)} of ${formatTokens(summary.monthly_grant)} plan tokens left, plus ${formatTokens(summary.topup_remaining)} top-up tokens`}
          >
            <div
              className={cn('h-full transition-[width] duration-700 ease-out', TONE_BAR[tone])}
              style={{ width: `${planPct}%` }}
            />
            {topupPct > 0 && (
              <div
                className="ml-0.5 h-full bg-accent transition-[width] duration-700 ease-out"
                style={{ width: `${topupPct}%` }}
              />
            )}
          </div>
          <dl className="mt-4 grid grid-cols-2 gap-4 text-body-sm">
            <div>
              <dt className="flex items-center gap-1.5 text-text-secondary">
                <span className={cn('h-2 w-2 rounded-full', TONE_BAR[tone])} /> Plan tokens
              </dt>
              <dd className="mt-0.5 tabular-nums text-text-primary">
                {formatTokens(summary.plan_grant_remaining)} / {formatTokens(summary.monthly_grant)}
              </dd>
              <dd className="text-caption text-text-tertiary">
                reset {formatShortDate(summary.next_grant_at)}
              </dd>
            </div>
            <div>
              <dt className="flex items-center gap-1.5 text-text-secondary">
                <span className="h-2 w-2 rounded-full bg-accent" /> Top-up tokens
              </dt>
              <dd className="mt-0.5 tabular-nums text-text-primary">
                {formatTokens(summary.topup_remaining)}
              </dd>
              {summary.topup_expires_at && (
                <dd className="text-caption text-text-tertiary">
                  until {formatShortDate(summary.topup_expires_at)}
                </dd>
              )}
            </div>
          </dl>
          <p className="mt-4 text-body-sm text-text-secondary">
            Used this cycle:{' '}
            <strong className="tabular-nums text-text-primary">
              {formatTokens(summary.used_this_cycle)}
            </strong>
          </p>
        </Card>

        <Card className="md:col-span-2">
          <p className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
            Plan
          </p>
          <CardTitle className="mt-1">{summary.plan.name}</CardTitle>
          <CardDescription className="mt-1 text-body-sm">
            {isPaid
              ? `${summary.plan.period === 'year' ? `${formatPrice(summary.plan.price_cents)}/year` : `${formatPrice(summary.plan.price_cents)}/month`} · ${summary.cancel_at_period_end ? 'ends' : 'renews'} ${formatShortDate(summary.period_end)}`
              : `${formatTokens(summary.monthly_grant)} tokens a month, free`}
          </CardDescription>
          {summary.cancel_at_period_end && (
            <p className="mt-3 rounded-md bg-warning/10 px-3 py-2 text-body-sm text-text-primary">
              Cancelled — Pro stays active until {formatShortDate(summary.period_end)}, then you
              move to Free.
            </p>
          )}
          <div className="mt-5 flex flex-col gap-2">
            <Link
              to="/pricing"
              className="rounded-md bg-accent px-3 py-2 text-center text-body-sm font-medium text-text-inverse hover:opacity-90"
            >
              {isPaid ? 'Change plan or buy tokens' : 'Upgrade to Pro'}
            </Link>
            {isPaid && isAdmin && (
              <Button
                variant="secondary"
                size="sm"
                loading={busy}
                onClick={() => void toggleCancel(!summary.cancel_at_period_end)}
              >
                {summary.cancel_at_period_end ? 'Resume subscription' : 'Cancel subscription'}
              </Button>
            )}
          </div>
        </Card>
      </div>

      <Card className="mt-6">
        <CardTitle>Where your tokens went</CardTitle>
        <CardDescription className="mt-1 text-body-sm">This cycle, after refunds.</CardDescription>
        {engineRows.length === 0 ? (
          <p className="mt-4 text-body-sm text-text-tertiary">No scans this cycle yet.</p>
        ) : (
          <div className="mt-4 grid grid-cols-1 gap-8 md:grid-cols-3">
            <ul className="space-y-2.5 md:col-span-2" aria-label="Tokens by engine">
              {engineRows.map(([engine, tokens]) => (
                <li
                  key={engine}
                  className="grid grid-cols-[6.5rem_1fr_4.5rem] items-center gap-3 text-body-sm"
                  title={`${ENGINE_META[engine as keyof typeof ENGINE_META]?.label ?? engine}: ${formatTokens(tokens)} tokens`}
                >
                  <span className="truncate text-text-secondary">
                    {ENGINE_META[engine as keyof typeof ENGINE_META]?.label ?? engine}
                  </span>
                  <span className="h-2.5 rounded-r bg-bg-subtle">
                    <span
                      className="block h-full rounded-r bg-accent"
                      style={{ width: `${(tokens / maxEngine) * 100}%` }}
                    />
                  </span>
                  <span className="text-right tabular-nums text-text-primary">
                    {formatTokens(tokens)}
                  </span>
                </li>
              ))}
            </ul>
            <dl className="space-y-2 text-body-sm" aria-label="Tokens by how the scan started">
              {(['manual', 'live', 'scheduled', 'cli_watch'] as const)
                .filter((k) => (byTrigger.get(k) ?? 0) > 0)
                .map((k) => (
                  <div
                    key={k}
                    className="flex justify-between border-b border-border-default pb-1.5"
                  >
                    <dt className="text-text-secondary">
                      {k === 'live'
                        ? 'Live scans'
                        : k === 'cli_watch'
                          ? 'CLI'
                          : k === 'manual'
                            ? 'Manual'
                            : 'Scheduled'}
                    </dt>
                    <dd className="tabular-nums text-text-primary">
                      {formatTokens(byTrigger.get(k) ?? 0)}
                    </dd>
                  </div>
                ))}
            </dl>
          </div>
        )}
      </Card>

      <Card className="mt-6 p-0">
        <div className="flex items-center justify-between p-6 pb-3">
          <div>
            <CardTitle>History</CardTitle>
            <CardDescription className="mt-1 text-body-sm">
              Every change to your balance, newest first.
            </CardDescription>
          </div>
          <Coins className="h-5 w-5 text-text-tertiary" aria-hidden="true" />
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-body-sm">
            <thead>
              <tr className="border-y border-border-default bg-bg-subtle text-left text-caption uppercase tracking-wide text-text-tertiary">
                <th className="px-6 py-2 font-semibold">When</th>
                <th className="px-3 py-2 font-semibold">What</th>
                <th className="px-3 py-2 text-right font-semibold">Tokens</th>
                <th className="px-6 py-2 text-right font-semibold">Balance</th>
              </tr>
            </thead>
            <tbody>
              {ledger.map((e) => (
                <tr key={e.id} className="border-b border-border-default last:border-0">
                  <td className="whitespace-nowrap px-6 py-2.5 text-text-secondary">
                    {formatDate(e.created_at)}
                  </td>
                  <td className="px-3 py-2.5 text-text-primary">
                    {ledgerLabel(e)}
                    {e.kind === 'refund' && e.reason && (
                      <span className="text-text-tertiary"> · {describeReason(e.reason)}</span>
                    )}
                    {e.kind === 'debit' && e.trigger_source && e.trigger_source !== 'manual' && (
                      <span className="ml-2 rounded bg-accent/10 px-1.5 py-0.5 text-caption text-accent">
                        {TRIGGER_LABEL[e.trigger_source] ?? e.trigger_source}
                      </span>
                    )}
                    {e.scan_id && (
                      <Link
                        to={`/scans/${e.scan_id}`}
                        className="ml-2 inline-flex items-center text-accent hover:underline"
                        aria-label="Open scan"
                      >
                        <ExternalLink className="h-3.5 w-3.5" />
                      </Link>
                    )}
                  </td>
                  <td
                    className={cn(
                      'whitespace-nowrap px-3 py-2.5 text-right font-medium tabular-nums',
                      e.delta < 0 ? 'text-danger' : 'text-success',
                    )}
                  >
                    {e.delta > 0 ? '+' : '−'}
                    {formatTokens(Math.abs(e.delta))}
                  </td>
                  <td className="whitespace-nowrap px-6 py-2.5 text-right tabular-nums text-text-secondary">
                    {formatTokens(e.balance_after)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {nextBefore && (
          <div className="p-4 text-center">
            <Button variant="ghost" size="sm" loading={loadingMore} onClick={() => void more()}>
              Show older
            </Button>
          </div>
        )}
      </Card>
    </main>
  )
}
