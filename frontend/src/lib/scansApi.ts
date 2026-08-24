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

export type PentestPreset = 'stealth' | 'standard' | 'deep' | 'custom'
export type PentestPortBreadth = 'top100' | 'top1000' | 'top1000_service_detect'
export type PentestWordlistTier = 'small' | 'medium' | 'large'

// PentestConfig mirrors dto.PentestConfigResponse — the resolved,
// already-clamped-to-the-ceiling scan-intensity config a pentest job
// actually ran (or will run) with.
export interface PentestConfig {
  preset: PentestPreset
  request_rate_per_sec: number
  port_breadth: PentestPortBreadth
  nuclei_categories: string[]
  phase_budget_seconds: number
  wordlist_tier: PentestWordlistTier
  subdomain_enum: boolean
  // Field names the server reduced to fit the operator-configured ceiling
  // — present only on the response to the CreateScan call itself.
  clamped_fields?: string[]
}

// PentestConfigInput mirrors dto.PentestConfigRequest — every field is
// optional, since a client normally sends only `preset` and lets every
// other value come from that preset's own bundle server-side.
export interface PentestConfigInput {
  preset?: PentestPreset
  request_rate_per_sec?: number
  port_breadth?: PentestPortBreadth
  nuclei_categories?: string[]
  phase_budget_seconds?: number
  wordlist_tier?: PentestWordlistTier
  subdomain_enum?: boolean
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
  // This scan's 1-based position among its own project's scans (oldest =
  // 1) — rendered as "Scan #N" in place of the raw UUID prefix.
  scan_number: number
  // null for a scan with no pentest job.
  pentest_config: PentestConfig | null
}

export interface EngineProgress {
  engine: Engine
  status: JobStatus
  progress_pct: number
  // A real, live "what's happening right now" label when the engine has
  // one (pentest's actual phase names, e.g. "Fuzzing for hidden files and
  // paths") — absent for engines with no named stages of their own.
  activity?: string
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
  // file — codescan, depscan, cicdscan, docreview. Also reused by the k8s
  // shape below (line_start/line_end against `file`, not `path`, there).
  path?: string
  line_start?: number
  line_end?: number
  // image — containerscan's Trivy-sourced vulnerability/secret findings.
  image?: string
  layer_digest?: string
  // k8s — k8sscan. from_helm/chart_name/template_file are set only when the
  // manifest came from rendering a Helm chart rather than a raw YAML file;
  // `file` is a real, navigable path either way (see repoLink.ts's usage).
  // `container` is set only for a per-container finding. `value` is the
  // literal offending value at field_path, e.g. "/var/run/docker.sock".
  file?: string
  kind?: string
  name?: string
  namespace?: string
  container?: string
  field_path?: string
  value?: string
  from_helm?: boolean
  chart_name?: string
  template_file?: string
  // dependency — depscan's version/CVE findings. Deliberately no line —
  // the vulnerability is in the resolved package's own code, not at a
  // specific line of this repository, so there is nothing to redirect to.
  ecosystem?: string
  package?: string
  version?: string
  manifest_path?: string
  // network — pentest. `url` is the literal endpoint the check actually
  // probed (scheme://host:port, or a full path for ffuf/nuclei-sourced
  // findings) — see internal/engines/pentest/findings.go's probedURL.
  host?: string
  ip?: string
  port?: number
  protocol?: string
  service?: string
  url?: string
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
  // "rule" or "ai" — cicdscan (Phase 10) is the first engine whose findings
  // can be AI-authored (its review_workflow semantic pass). Always "rule"
  // for every earlier engine.
  source: 'rule' | 'ai' | string
  // Engine-specific extra context, never load-bearing — today: k8sscan's
  // and containerscan's optional `impact` (a short why-this-matters
  // sentence) and `attack_path` (an ordered escalation-chain string
  // array), see FindingRow's AttackContext. Absent keys are absent.
  metadata?: {
    impact?: string
    attack_path?: string[]
    [key: string]: unknown
  }
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
  // This scan's 1-based position among its own project's scans (oldest =
  // 1) — rendered as "Scan #N" in place of the raw UUID prefix.
  scan_number: number
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
  // Ignored server-side unless the resolved engine set includes pentest.
  // Omitted entirely means "run pentest at the literal Stealth default" —
  // the same server-side fallback a raw API call with no body gets.
  pentest_config?: PentestConfigInput
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

export type ExportFormat = 'json' | 'csv' | 'pdf'

/** Downloads a scan report (`GET /scans/{id}/export?format=…`) and triggers
 * the browser's native save — a self-contained report snapshot (coverage,
 * every finding, an AI-authored executive summary when available), distinct
 * from the live paginated getScan/listFindings above. */
export async function exportScan(scanId: string, format: ExportFormat): Promise<void> {
  const { blob, filename } = await apiClient.download(`/scans/${scanId}/export?format=${format}`)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Revoked on a delay, not immediately — revoking synchronously right
  // after click() can race the browser's own download start in some
  // browsers, silently producing an empty/failed download.
  setTimeout(() => URL.revokeObjectURL(url), 10_000)
}
