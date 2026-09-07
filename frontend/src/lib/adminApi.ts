/**
 * Typed wrappers around apiClient for the platform admin panel
 * (BUILD_GUIDE.md Phase 14) — not yet in documentation/07-api-specification.md,
 * see internal/transport/http/dto/admin.go's own doc-debt note. Matches
 * rulesApi.ts's plain-functions-over-Zustand pattern for server state.
 */

import { apiClient } from "./apiClient";
import type { Pagination } from "./rulesApi";

export interface OrganizationSummary {
  id: string;
  name: string;
  member_count: number;
  project_count: number;
  scan_count: number;
  suspended_at: string | null;
  suspended_reason: string | null;
  created_at: string;
}

export interface UserSummary {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  role: "admin" | "member" | "viewer";
  suspended_at: string | null;
  suspended_reason: string | null;
  last_login_at: string | null;
  created_at: string;
}

export interface OrganizationDetail extends OrganizationSummary {
  members: UserSummary[];
}

export interface OrganizationList {
  data: OrganizationSummary[];
  pagination: Pagination;
}

export function listOrganizations(search = ""): Promise<OrganizationList> {
  const params = new URLSearchParams();
  if (search) params.set("search", search);
  params.set("page_size", "100");
  const query = params.toString();
  return apiClient.get<OrganizationList>(
    `/admin/organizations${query ? `?${query}` : ""}`,
  );
}

export function getOrganization(id: string): Promise<OrganizationDetail> {
  return apiClient.get<OrganizationDetail>(
    `/admin/organizations/${encodeURIComponent(id)}`,
  );
}

export function suspendOrganization(
  id: string,
  reason: string,
): Promise<OrganizationDetail> {
  return apiClient.post<OrganizationDetail>(
    `/admin/organizations/${encodeURIComponent(id)}/suspend`,
    { reason },
  );
}

export function reinstateOrganization(id: string): Promise<OrganizationDetail> {
  return apiClient.post<OrganizationDetail>(
    `/admin/organizations/${encodeURIComponent(id)}/reinstate`,
  );
}

export function suspendUser(id: string, reason: string): Promise<void> {
  return apiClient.post<void>(
    `/admin/users/${encodeURIComponent(id)}/suspend`,
    { reason },
  );
}

export function reinstateUser(id: string): Promise<void> {
  return apiClient.post<void>(
    `/admin/users/${encodeURIComponent(id)}/reinstate`,
  );
}

// --- pentest misuse flags ---

export type FlagStatus =
  "open" | "investigating" | "dismissed" | "confirmed_misuse";
export type FlagSource =
  "self_reported" | "external_complaint" | "operator_review";

export interface TargetInfo {
  target_id: string;
  host: string;
  project_id: string;
  project_name: string;
  org_id: string;
  org_name: string;
}

export interface PentestFlag {
  id: string;
  status: FlagStatus;
  source: FlagSource;
  reason: string;
  reported_by: string | null;
  resolved_by: string | null;
  resolved_at: string | null;
  created_at: string;
  target: TargetInfo;
}

export interface PentestFlagList {
  data: PentestFlag[];
  pagination: Pagination;
}

export function createPentestFlag(input: {
  target_id: string;
  source: FlagSource;
  reason: string;
}): Promise<{ id: string; status: FlagStatus }> {
  return apiClient.post(`/admin/pentest-flags`, input);
}

export function listPentestFlags(
  status?: FlagStatus,
): Promise<PentestFlagList> {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  params.set("page_size", "100");
  const query = params.toString();
  return apiClient.get<PentestFlagList>(
    `/admin/pentest-flags${query ? `?${query}` : ""}`,
  );
}

export function resolvePentestFlag(
  id: string,
  status: Exclude<FlagStatus, "open">,
): Promise<{ id: string; status: FlagStatus }> {
  return apiClient.patch(`/admin/pentest-flags/${encodeURIComponent(id)}`, {
    status,
  });
}

// --- audit log ---

export interface AuditEntry {
  id: number;
  org_id: string | null;
  actor_id: string | null;
  action: string;
  resource_type: string | null;
  resource_id: string | null;
  detail: Record<string, unknown>;
  ip: string | null;
  created_at: string;
}

export interface AuditLogList {
  data: AuditEntry[];
  pagination: Pagination;
}

export function listAuditLog(
  filters: { orgId?: string; action?: string } = {},
): Promise<AuditLogList> {
  const params = new URLSearchParams();
  if (filters.orgId) params.set("org_id", filters.orgId);
  if (filters.action) params.set("action", filters.action);
  params.set("page_size", "50");
  const query = params.toString();
  return apiClient.get<AuditLogList>(
    `/admin/audit-log${query ? `?${query}` : ""}`,
  );
}

// --- system health ---

export interface EngineJobStats {
  engine: string;
  succeeded: number;
  failed: number;
  skipped: number;
}

export interface SystemHealth {
  engine_stats: EngineJobStats[];
  jobs_in_flight: number;
  sandbox_containers_running: number | null;
  gemini: { available: boolean; pool_size: number; current_index: number };
  ai_cache: { available: boolean; hits: number; misses: number };
  checked_at: string;
}

export function getSystemHealth(): Promise<SystemHealth> {
  return apiClient.get<SystemHealth>("/admin/system-health");
}
