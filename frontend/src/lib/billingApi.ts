/**
 * Typed wrappers around apiClient for token billing
 * (TOKENIZATION-ARCHITECTURE.md). The frontend never keeps its own copy of
 * a price: everything shown comes from /billing/catalog, /billing/estimate
 * or /billing/summary, so the UI can't drift from what the backend charges.
 */

import { apiClient, ApiError } from './apiClient'
import type { Engine } from './rulesApi'
import type { PentestConfigInput, ScanType, TriggerSource } from './scansApi'

export type PlanCode = 'free' | 'pro_monthly' | 'pro_annual'

export interface BillingPlan {
  code: PlanCode
  name: string
  price_cents: number
  period: 'month' | 'year'
  monthly_grant: number
  engines: Engine[]
  live_scanning: boolean
  schedules: boolean
  topup_discount_percent: number
}

export interface BillingPack {
  code: string
  name: string
  tokens: number
  price_cents: number
}

export interface BillingExample {
  key: string
  label: string
  tokens: number
  per_pro_month: number
  engines: Engine[]
}

export interface BillingCatalog {
  price_version: string
  plans: BillingPlan[]
  packs: BillingPack[]
  engine_prices: Record<Engine, number>
  pentest_multipliers: Record<string, number>
  live_discount: number
  // Finding-assistant command prices (explain / remediate / fix). Absent
  // from an older backend.
  ai_prices?: Record<string, number>
  examples: BillingExample[]
  guarantee: {
    pentests: number
    full_scans: number
    live_pushes: number
    tokens_needed: number
    monthly_allowance: number
  }
}

export interface UsageRow {
  engine: Engine
  trigger_source: TriggerSource | string
  tokens: number
}

export interface BillingSummary {
  plan: BillingPlan
  status: string
  period_start: string
  period_end: string
  next_grant_at: string
  cancel_at_period_end: boolean
  balance: number
  plan_grant_remaining: number
  topup_remaining: number
  topup_expires_at: string | null
  monthly_grant: number
  used_this_cycle: number
  usage: UsageRow[]
  mode: 'demo' | 'stripe' | 'off'
}

export type LedgerKind = 'grant' | 'topup' | 'debit' | 'refund' | 'expire' | 'adjustment'

export interface LedgerEntry {
  id: string
  kind: LedgerKind
  delta: number
  balance_after: number
  scan_id: string | null
  engine: Engine | null
  trigger_source: string | null
  reason: string | null
  created_at: string
}

export interface LedgerPage {
  data: LedgerEntry[]
  next_before: string | null
}

export interface ScanTokens {
  charged: number
  refunded: number
  entries: LedgerEntry[]
}

export interface EstimateRequest {
  project_id?: string
  type?: ScanType
  engines?: Engine[]
  pentest_config?: PentestConfigInput
  trigger?: TriggerSource
}

export interface Estimate {
  engines: Engine[]
  total: number
  lines: { engine: Engine; tokens: number; full_tokens: number }[]
  live_discount: boolean
  balance: number
  balance_after: number
  affordable: boolean
  plan_allowed: boolean
  blocked_engines: Engine[]
  required_plan: PlanCode | null
}

export interface CheckoutSession {
  id: string
  item_code: string
  item_name: string
  item_kind: 'plan' | 'pack'
  tokens: number
  amount_cents: number
  currency: string
  provider: string
  status: 'pending' | 'paid' | 'failed' | 'expired'
  expires_at: string
  paid_at: string | null
  redirect_url?: string
}

export function getCatalog(): Promise<BillingCatalog> {
  return apiClient.get<BillingCatalog>('/billing/catalog')
}

export function getBillingSummary(): Promise<BillingSummary> {
  return apiClient.get<BillingSummary>('/billing/summary')
}

export function getLedger(limit = 25, before?: string): Promise<LedgerPage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (before) params.set('before', before)
  return apiClient.get<LedgerPage>(`/billing/ledger?${params.toString()}`)
}

export function getScanTokens(scanId: string): Promise<ScanTokens> {
  return apiClient.get<ScanTokens>(`/billing/scans/${scanId}`)
}

export function estimateScan(req: EstimateRequest): Promise<Estimate> {
  return apiClient.post<Estimate>('/billing/estimate', req)
}

export function startCheckout(itemCode: string): Promise<CheckoutSession> {
  return apiClient.post<CheckoutSession>('/billing/checkout', { item_code: itemCode })
}

export function getCheckout(id: string): Promise<CheckoutSession> {
  return apiClient.get<CheckoutSession>(`/billing/checkout/${id}`)
}

/** Demo mode only. Sends the outcome and nothing else — never card data. */
export function confirmCheckout(
  id: string,
  outcome: 'success' | 'declined',
): Promise<CheckoutSession> {
  return apiClient.post<CheckoutSession>(`/billing/checkout/${id}/confirm`, { outcome })
}

export function cancelSubscription(): Promise<BillingSummary> {
  return apiClient.post<BillingSummary>('/billing/subscription/cancel', {})
}

export function resumeSubscription(): Promise<BillingSummary> {
  return apiClient.post<BillingSummary>('/billing/subscription/resume', {})
}

// --- formatting ---

const NUM = new Intl.NumberFormat('en')

export function formatTokens(n: number): string {
  return NUM.format(Math.round(n))
}

/** 500000 → "500k", 15000 → "15k", 3250 → "3.25k". */
export function formatTokensShort(n: number): string {
  if (Math.abs(n) >= 1_000_000) return `${+(n / 1_000_000).toFixed(2)}M`
  if (Math.abs(n) >= 1_000) return `${+(n / 1_000).toFixed(2)}k`
  return String(n)
}

export function formatPrice(cents: number): string {
  return cents % 100 === 0 ? `$${cents / 100}` : `$${(cents / 100).toFixed(2)}`
}

export function formatShortDate(iso: string): string {
  return new Intl.DateTimeFormat('en', { day: 'numeric', month: 'short', year: 'numeric' }).format(
    new Date(iso),
  )
}

/** A billing error's user-facing sentence, or null if err isn't one. */
export function billingErrorMessage(err: unknown): string | null {
  if (!(err instanceof ApiError)) return null
  const p = err.problem
  switch (p.code) {
    case 'billing.insufficient_tokens':
      return `This needs ${formatTokens(p.required ?? 0)} tokens and you have ${formatTokens(p.available ?? 0)}.`
    case 'billing.plan_required':
      return `${p.detail}. Upgrade to Pro to unlock it.`
    case 'billing.already_subscribed':
    case 'billing.payment_declined':
    case 'billing.checkout_expired':
    case 'billing.checkout_closed':
    case 'billing.admin_required':
    case 'billing.nothing_to_cancel':
      return p.detail
  }
  return null
}

/** Ledger reason codes → plain words. */
export function describeReason(reason: string | null): string {
  if (!reason) return ''
  if (reason.startsWith('engine_failed')) return 'engine failed — refunded'
  const map: Record<string, string> = {
    engine_not_applicable: 'engine not applicable — refunded',
    cancelled_before_start: 'cancelled before it started — refunded',
    create_failed: 'scan could not be created — refunded',
    welcome: 'Free plan',
    purchase: 'plan purchase',
    monthly_grant: 'monthly tokens',
    plan_period_ended: 'unused plan tokens',
    expired: 'expired',
    topup_100k: '100k top-up',
    topup_250k: '250k top-up',
    topup_600k: '600k top-up',
  }
  return map[reason] ?? reason
}

export const TRIGGER_LABEL: Record<string, string> = {
  manual: 'Manual',
  scheduled: 'Scheduled',
  webhook_push: 'Live · push',
  webhook_pull_request: 'Live · PR',
  cli_watch: 'CLI',
}
