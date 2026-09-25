/**
 * The finding assistant (`POST /findings/{id}/assist`, modules/assist on
 * the backend). Commands only — there is no free-text chat. Prices come
 * from the billing catalogue (`ai_prices`), never hard-coded here.
 */

import { apiClient } from './apiClient'

export type AssistAction = 'explain' | 'remediate' | 'fix'

export interface AssistExplain {
  what: string
  why_it_matters: string
  how_exploited: string
  confidence: 'high' | 'medium' | 'low'
}

export interface AssistStep {
  title: string
  detail: string
  code?: string
}

export interface AssistRemediate {
  summary: string
  steps: AssistStep[]
  verification: string
  confidence: 'high' | 'medium' | 'low'
}

export interface AssistFix {
  patch: string
  explanation: string
  confidence: 'high' | 'medium' | 'low'
  caveats: string[]
}

export interface AssistResponse {
  action: AssistAction
  tokens_charged: number
  already_paid: boolean
  source_used: boolean
  explain?: AssistExplain
  remediate?: AssistRemediate
  fix?: AssistFix
}

export function runAssist(findingId: string, action: AssistAction): Promise<AssistResponse> {
  return apiClient.post<AssistResponse>(`/findings/${findingId}/assist`, { action })
}
