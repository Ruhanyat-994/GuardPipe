import { useCallback, useEffect, useState } from 'react'
import { Coins, FastForward } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { Input } from '../ui/Input'
import { apiClient, ApiError } from '../../lib/apiClient'
import { cn } from '../../lib/cn'
import { formatDate } from '../../lib/format'
import {
  formatShortDate,
  formatTokens,
  type BillingSummary,
  type LedgerEntry,
} from '../../lib/billingApi'
import { ledgerLabel } from '../../lib/billingUi'

interface AdminBilling {
  summary: BillingSummary
  ledger: LedgerEntry[]
}

/**
 * Operator view of one organisation's billing: plan, balance, recent
 * ledger, a goodwill/correction token adjustment, and (demo mode only)
 * "advance cycle" to show a monthly renewal live.
 */
export function AdminBillingCard({ orgId }: { orgId: string }) {
  const [data, setData] = useState<AdminBilling | null>(null)
  const [unavailable, setUnavailable] = useState(false)
  const [delta, setDelta] = useState('')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState<'adjust' | 'advance' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(() => {
    apiClient
      .get<AdminBilling>(`/admin/billing/orgs/${orgId}`)
      .then(setData)
      .catch((err) => {
        if (err instanceof ApiError && err.problem.status === 404) setUnavailable(true)
      })
  }, [orgId])
  useEffect(load, [load])

  async function run(kind: 'adjust' | 'advance') {
    setBusy(kind)
    setError(null)
    try {
      const res =
        kind === 'adjust'
          ? await apiClient.post<AdminBilling>(`/admin/billing/orgs/${orgId}/adjust`, {
              delta: Number(delta),
              reason,
            })
          : await apiClient.post<AdminBilling>(`/admin/billing/orgs/${orgId}/advance-cycle`, {})
      setData(res)
      setDelta('')
      setReason('')
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Request failed.')
    } finally {
      setBusy(null)
    }
  }

  if (unavailable) return null
  const s = data?.summary

  return (
    <Card className="mb-4">
      <CardTitle className="flex items-center gap-2 text-h3">
        <Coins className="h-4 w-4" aria-hidden="true" /> Billing
      </CardTitle>
      {!s ? (
        <CardDescription className="mt-2">Loading…</CardDescription>
      ) : (
        <>
          <dl className="mt-3 grid grid-cols-2 gap-3 text-body-sm sm:grid-cols-4">
            <div>
              <dt className="text-text-tertiary">Plan</dt>
              <dd className="text-text-primary">
                {s.plan.name}
                {s.cancel_at_period_end && ' (cancelled)'}
              </dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Balance</dt>
              <dd className="tabular-nums text-text-primary">{formatTokens(s.balance)}</dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Used this cycle</dt>
              <dd className="tabular-nums text-text-primary">{formatTokens(s.used_this_cycle)}</dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Next grant</dt>
              <dd className="text-text-primary">{formatShortDate(s.next_grant_at)}</dd>
            </div>
          </dl>

          <div className="mt-4 flex flex-wrap items-end gap-2">
            <label className="text-body-sm text-text-secondary">
              Tokens (±)
              <Input
                className="mt-1 w-32"
                type="number"
                value={delta}
                onChange={(e) => setDelta(e.target.value)}
                placeholder="100000"
              />
            </label>
            <label className="flex-1 text-body-sm text-text-secondary">
              Reason
              <Input
                className="mt-1"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="Goodwill credit"
              />
            </label>
            <Button
              size="sm"
              loading={busy === 'adjust'}
              disabled={!delta || Number(delta) === 0 || !reason}
              onClick={() => void run('adjust')}
            >
              Adjust tokens
            </Button>
            {s.mode === 'demo' && (
              <Button
                size="sm"
                variant="secondary"
                loading={busy === 'advance'}
                onClick={() => void run('advance')}
                title="Demo mode: run the next monthly renewal now"
              >
                <FastForward className="h-4 w-4" aria-hidden="true" /> Advance cycle (demo)
              </Button>
            )}
          </div>
          {error && <p className="mt-2 text-body-sm text-danger">{error}</p>}

          <ul className="mt-4 divide-y divide-border-default text-body-sm">
            {data.ledger.map((e) => (
              <li key={e.id} className="flex justify-between gap-3 py-1.5">
                <span className="min-w-0 truncate text-text-secondary">
                  {ledgerLabel(e)}{' '}
                  <span className="text-text-tertiary">· {formatDate(e.created_at)}</span>
                </span>
                <span
                  className={cn(
                    'shrink-0 tabular-nums',
                    e.delta < 0 ? 'text-danger' : 'text-success',
                  )}
                >
                  {e.delta > 0 ? '+' : '−'}
                  {formatTokens(Math.abs(e.delta))}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  )
}
