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
  image?: string
  layer_digest?: string
  [key: string]: unknown
}

export interface Evidence {
  kind: string
  value: string
  redacted: boolean
  line_start?: number
  line_end?: number
}

export interface FindingListItem {
  id: string
  engine: Engine
  rule_id: string
  title: string
  description: string
  remediation: string
  severity: Severity
  confidence: string
  status: string
  cwe: string[]
  cve: string[]
  owasp: string[]
  cvss_score: number | null
  location: Location
  evidence: Evidence[]
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

// ScanSummary is one row of the scan-history table — deliberately lighter
// than Scan (no per-job detail; `GET /projects/{id}/scans` doesn't return
// it, avoiding an N+1 job/finding-count query per row for a list endpoint).
export interface ScanSummary {
  id: string
  project_id: string
  type: ScanType
  status: ScanStatus
  branch: string | null
  queued_at: string
  started_at: string | null
  finished_at: string | null
  finding_counts: Record<string, number>
}

export interface ScanList {
  data: ScanSummary[]
  pagination: Pagination
}

// OrgScanSummary is one row of the global, cross-project scan history —
// ScanSummary plus the project name, so the page doesn't need a second
// request per row to know whose scan it's showing.
export interface OrgScanSummary extends ScanSummary {
  project_name: string
}

export interface OrgScanList {
  data: OrgScanSummary[]
  pagination: Pagination
}

export interface CreateScanInput {
  type: ScanType
  // Only used (and only meaningful) when type is 'partial' — the backend
  // rejects 'partial' with no engines, and ignores this field otherwise
  // (documentation/07-api-specification.md §5: 'full_supply_chain' always
  // runs every registered engine).
  engines?: Engine[]
}

export function createScan(
  projectId: string,
  input: CreateScanInput = { type: 'full_supply_chain' },
): Promise<Scan> {
  return apiClient.post<Scan>(`/projects/${projectId}/scans`, input)
}

export function listScans(projectId: string, page = 1, pageSize = 20): Promise<ScanList> {
  return apiClient.get<ScanList>(`/projects/${projectId}/scans?page=${page}&page_size=${pageSize}`)
}

export function listOrgScans(page = 1, pageSize = 20): Promise<OrgScanList> {
  return apiClient.get<OrgScanList>(`/scans?page=${page}&page_size=${pageSize}`)
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
