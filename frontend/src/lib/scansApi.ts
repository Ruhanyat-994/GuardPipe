/**
 * Typed wrappers around apiClient for the scan/finding endpoints
 * (documentation/07-api-specification.md §5-6, the subset BUILD_GUIDE.md
 * Phase 6 builds) — matching projectsApi.ts's plain-functions pattern.
 */

import { apiClient } from './apiClient'
import type { Engine, Severity } from './rulesApi'

export type ScanType = 'full_supply_chain' | 'partial' | 'pentest_only'
export type ScanStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'skipped' | 'cancelled'

export interface Job {
  id: string
  engine: Engine
  status: JobStatus
  started_at: string | null
  finished_at: string | null
  finding_count: number
  stats: Record<string, unknown> | null
  error_reason: string | null
  skip_reason: string | null
}

export interface Scan {
  id: string
  project_id: string
  type: ScanType
  status: ScanStatus
  requested_engines: Engine[]
  branch: string | null
  commit_sha: string | null
  queued_at: string
  started_at: string | null
  finished_at: string | null
  finding_counts: Record<string, number>
  risk: unknown
  jobs: Job[]
}

export interface EngineProgress {
  engine: Engine
  status: JobStatus
  progress_pct: number
  finding_count: number
}

export interface Progress {
  scan_id: string
  status: ScanStatus
  progress_pct: number
  engines: EngineProgress[]
}

export interface Location {
  type: string
  path?: string
  line_start?: number
  line_end?: number
  ecosystem?: string
  package?: string
  version?: string
  manifest_path?: string
  [key: string]: unknown
}

export interface FindingListItem {
  id: string
  engine: Engine
  rule_id: string
  title: string
  severity: Severity
  confidence: string
  status: string
  cwe: string[]
  cve: string[]
  owasp: string[]
  cvss_score: number | null
  location: Location
}

export interface Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface FindingList {
  data: FindingListItem[]
  pagination: Pagination
}

export function createScan(projectId: string, type: ScanType = 'full_supply_chain'): Promise<Scan> {
  return apiClient.post<Scan>(`/projects/${projectId}/scans`, { type })
}

export function getScan(scanId: string): Promise<Scan> {
  return apiClient.get<Scan>(`/scans/${scanId}`)
}

export function getProgress(scanId: string): Promise<Progress> {
  return apiClient.get<Progress>(`/scans/${scanId}/progress`)
}

export function cancelScan(scanId: string): Promise<void> {
  return apiClient.post<void>(`/scans/${scanId}/cancel`)
}

export function listFindings(scanId: string): Promise<FindingList> {
  return apiClient.get<FindingList>(`/scans/${scanId}/findings?page_size=100`)
}
