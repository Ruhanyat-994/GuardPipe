import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Coins } from 'lucide-react'
import { Popover } from '../ui/Popover'
import { cn } from '../../lib/cn'
import { useCountUp } from '../../hooks/useCountUp'
import { useBillingStore } from '../../stores/billingStore'
import { useAuthStore } from '../../stores/authStore'
import { formatShortDate, formatTokens, getLedger, type LedgerEntry } from '../../lib/billingApi'
import { ledgerLabel, meterTone, TONE_BAR } from '../../lib/billingUi'
import { relativeTime } from '../../lib/format'

const POLL_MS = 30_000

/**
 * The always-visible token balance in the top bar: an animated count and a
 * thin meter that drains as scans spend tokens and refills when a job is
 * refunded or tokens are bought. Each change floats away as a small
 * "−20,500" / "+4,000" chip. Click for the breakdown and recent activity.
 */
export function TokenBar() {
  const summary = useBillingStore((s) => s.summary)
  const unavailable = useBillingStore((s) => s.unavailable)
  const lastChange = useBillingStore((s) => s.lastChange)
  const refresh = useBillingStore((s) => s.refresh)
  const reset = useBillingStore((s) => s.reset)
  const orgId = useAuthStore((s) => s.user?.orgId)
  const scopedProjectId = useAuthStore((s) => s.user?.scopedProjectId)

  // Re-fetch on org switch; poll only while the tab is visible.
  useEffect(() => {
    reset()
    if (!orgId || scopedProjectId) return
    void refresh()
    const id = window.setInterval(() => {
      if (document.visibilityState === 'visible') void refresh()
    }, POLL_MS)
    return () => window.clearInterval(id)
  }, [orgId, scopedProjectId, refresh, reset])

  // Floating change chips.
  const [chips, setChips] = useState<{ id: number; delta: number }[]>([])
  const seen = useRef(0)
  useEffect(() => {
    if (!lastChange || lastChange.id === seen.current) return
    seen.current = lastChange.id
    setChips((c) => [...c, lastChange])
    const t = window.setTimeout(
      () => setChips((c) => c.filter((x) => x.id !== lastChange.id)),
      1900,
    )
    return () => window.clearTimeout(t)
  }, [lastChange])

  const shown = useCountUp(summary?.balance ?? 0)

  if (unavailable || scopedProjectId || !summary) return null

  const grant = Math.max(summary.monthly_grant, 1)
  const pct = Math.min(100, (summary.balance / grant) * 100)
  const tone = meterTone(summary.balance, summary.monthly_grant)
  const isPro = summary.plan.code !== 'free'

  return (
    <Popover
      align="right"
      panelClassName="w-80"
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={toggle}
          aria-expanded={open}
          aria-label={`${formatTokens(summary.balance)} tokens left`}
          className={cn(
            'relative flex items-center gap-2 rounded-full border border-chrome-border px-3 py-1 text-body-sm text-chrome-text transition-colors hover:bg-chrome-hover',
            open && 'bg-chrome-hover',
          )}
        >
          <Coins
            className={cn(
              'h-4 w-4',
              tone === 'critical'
                ? 'text-danger'
                : tone === 'low'
                  ? 'text-warning'
                  : 'text-success',
            )}
            aria-hidden="true"
          />
          <span className="font-semibold tabular-nums">{formatTokens(shown)}</span>
          <span className="hidden text-chrome-text-secondary sm:inline">tokens</span>
          <span className="relative hidden h-1.5 w-20 overflow-hidden rounded-full bg-chrome-border md:block">
            <span
              className={cn(
                'absolute inset-y-0 left-0 rounded-full transition-[width] duration-700 ease-out',
                TONE_BAR[tone],
                tone === 'critical' && 'animate-pulse',
              )}
              style={{ width: `${pct}%` }}
            />
            {lastChange && (
              <span
                key={lastChange.id}
                className="token-shimmer absolute inset-y-0 w-1/3 bg-gradient-to-r from-transparent via-white/40 to-transparent"
              />
            )}
          </span>
          <span
            className={cn(
              'rounded px-1.5 py-0.5 text-caption font-semibold uppercase',
              isPro ? 'bg-accent/20 text-accent' : 'bg-chrome-border text-chrome-text-secondary',
            )}
          >
            {isPro ? 'Pro' : 'Free'}
          </span>
          {chips.map((c) => (
            <span
              key={c.id}
              aria-hidden="true"
              className={cn(
                'token-float pointer-events-none absolute -bottom-5 left-8 whitespace-nowrap rounded px-1.5 text-caption font-semibold tabular-nums',
                c.delta < 0 ? 'text-danger' : 'text-success',
              )}
            >
              {c.delta > 0 ? '+' : '−'}
              {formatTokens(Math.abs(c.delta))}
            </span>
          ))}
        </button>
      )}
    >
      {(close) => <TokenPopover close={close} />}
    </Popover>
  )
}

function TokenPopover({ close }: { close: () => void }) {
  const summary = useBillingStore((s) => s.summary)
  const [recent, setRecent] = useState<LedgerEntry[] | null>(null)

  useEffect(() => {
    getLedger(5)
      .then((res) => setRecent(res.data))
      .catch(() => setRecent([]))
  }, [])

  if (!summary) return null
  const planPct = Math.min(
    100,
    (summary.plan_grant_remaining / Math.max(summary.monthly_grant, 1)) * 100,
  )
  const tone = meterTone(summary.balance, summary.monthly_grant)

  return (
    <div className="p-4 text-text-primary">
      <div className="flex items-baseline justify-between">
        <span className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
          {summary.plan.name} plan
        </span>
        <span className="text-caption text-text-tertiary">
          {summary.cancel_at_period_end ? 'ends' : 'resets'}{' '}
          {formatShortDate(summary.next_grant_at)}
        </span>
      </div>
      <p className="mt-1 text-h2 font-semibold tabular-nums">
        {formatTokens(summary.balance)}{' '}
        <span className="text-body-sm font-normal text-text-secondary">tokens</span>
      </p>

      <div className="mt-3 space-y-2 text-body-sm">
        <div>
          <div className="flex justify-between">
            <span className="text-text-secondary">Plan tokens</span>
            <span className="tabular-nums">
              {formatTokens(summary.plan_grant_remaining)} / {formatTokens(summary.monthly_grant)}
            </span>
          </div>
          <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-bg-subtle">
            <div
              className={cn('h-full rounded-full', TONE_BAR[tone])}
              style={{ width: `${planPct}%` }}
            />
          </div>
        </div>
        {summary.topup_remaining > 0 && (
          <div className="flex justify-between">
            <span className="text-text-secondary">Top-up tokens</span>
            <span className="tabular-nums">
              {formatTokens(summary.topup_remaining)}
              {summary.topup_expires_at && (
                <span className="text-text-tertiary">
                  {' '}
                  · until {formatShortDate(summary.topup_expires_at)}
                </span>
              )}
            </span>
          </div>
        )}
      </div>

      <div className="mt-4 border-t border-border-default pt-3">
        <p className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
          Recent
        </p>
        {recent === null ? (
          <p className="mt-2 text-body-sm text-text-tertiary">Loading…</p>
        ) : recent.length === 0 ? (
          <p className="mt-2 text-body-sm text-text-tertiary">No activity yet.</p>
        ) : (
          <ul className="mt-2 space-y-1.5">
            {recent.map((e) => (
              <li key={e.id} className="flex items-center justify-between gap-2 text-body-sm">
                <span className="min-w-0 truncate text-text-secondary">
                  {ledgerLabel(e)}
                  <span className="text-text-tertiary"> · {relativeTime(e.created_at)}</span>
                </span>
                <span
                  className={cn(
                    'shrink-0 font-medium tabular-nums',
                    e.delta < 0 ? 'text-danger' : 'text-success',
                  )}
                >
                  {e.delta > 0 ? '+' : '−'}
                  {formatTokens(Math.abs(e.delta))}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="mt-4 flex gap-2">
        <Link
          to="/settings/billing"
          onClick={close}
          className="flex-1 rounded-md border border-border-default px-3 py-1.5 text-center text-body-sm font-medium hover:bg-bg-subtle"
        >
          View usage
        </Link>
        <Link
          to="/pricing"
          onClick={close}
          className="flex-1 rounded-md bg-accent px-3 py-1.5 text-center text-body-sm font-medium text-text-inverse hover:opacity-90"
        >
          Buy tokens
        </Link>
      </div>
    </div>
  )
}
