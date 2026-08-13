/**
 * Typed wrappers around apiClient for the rules catalogue endpoints
 * (documentation/07-api-specification.md §8), matching projectsApi.ts's
 * plain-functions-over-Zustand pattern for server state.
 */

import { apiClient } from './apiClient'

export type Engine =
  'docreview' | 'codescan' | 'depscan' | 'containerscan' | 'k8sscan' | 'cicdscan' | 'pentest'

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'informational'
export type Tier = 'core' | 'stretch'

export interface Rule {
  id: string
  engine: Engine
  category: string
  title: string
  description: string
  remediation: string
  default_severity: Severity
  cwe: string[]
  owasp: string[]
  references: string[]
  tier: Tier
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface RuleList {
  data: Rule[]
  pagination: Pagination
}

export interface RuleFilters {
  engine?: Engine
  tier?: Tier
  severity?: Severity
}

export function listRules(filters: RuleFilters = {}): Promise<RuleList> {
  const params = new URLSearchParams()
  if (filters.engine) params.set('engine', filters.engine)
  if (filters.tier) params.set('tier', filters.tier)
  if (filters.severity) params.set('severity', filters.severity)
  params.set('page_size', '200') // the whole catalogue is small enough not to need pagination UI yet

  const query = params.toString()
  return apiClient.get<RuleList>(`/rules${query ? `?${query}` : ''}`)
}

export function getRule(id: string): Promise<Rule> {
  return apiClient.get<Rule>(`/rules/${encodeURIComponent(id)}`)
}

export function setRuleEnabled(id: string, enabled: boolean): Promise<Rule> {
  return apiClient.patch<Rule>(`/rules/${encodeURIComponent(id)}`, { enabled })
}
