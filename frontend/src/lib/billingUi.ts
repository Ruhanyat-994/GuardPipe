/** Display helpers shared by the token bar, Billing page and admin card. */

import { ENGINE_META } from './engines'
import { describeReason, TRIGGER_LABEL, type LedgerEntry } from './billingApi'

/** Colour of the meter by what's left of the monthly grant. */
export function meterTone(balance: number, monthlyGrant: number): 'ok' | 'low' | 'critical' {
  const pct = monthlyGrant > 0 ? balance / monthlyGrant : 1
  if (pct < 0.1) return 'critical'
  if (pct < 0.25) return 'low'
  return 'ok'
}

export const TONE_BAR: Record<ReturnType<typeof meterTone>, string> = {
  ok: 'bg-success',
  low: 'bg-warning',
  critical: 'bg-danger',
}

const AI_COMMAND_LABEL: Record<string, string> = {
  explain: 'Define it',
  remediate: 'Remediation',
  fix: 'Fix it',
}

/** One ledger row as a short phrase: "Code scan · Live · push". */
export function ledgerLabel(e: LedgerEntry): string {
  const engine = e.engine ? (ENGINE_META[e.engine]?.label ?? e.engine) : ''
  switch (e.kind) {
    case 'debit':
      if (e.reason?.startsWith('ai_assist:')) {
        const cmd = e.reason.slice('ai_assist:'.length)
        return `GuardPipe AI · ${AI_COMMAND_LABEL[cmd] ?? cmd}`
      }
      return `${engine} scan${e.trigger_source && e.trigger_source !== 'manual' ? ` · ${TRIGGER_LABEL[e.trigger_source] ?? e.trigger_source}` : ''}`
    case 'refund':
      return `Refund · ${engine}`
    case 'grant':
      return `Monthly tokens${e.reason === 'purchase' ? ' · plan purchase' : ''}`
    case 'topup':
      return 'Top-up purchase'
    case 'expire':
      return `Expired · ${describeReason(e.reason)}`
    case 'adjustment':
      return `Adjustment${e.reason ? ` · ${e.reason}` : ''}`
  }
}
