/**
 * Typed wrappers around apiClient for the `project` endpoints
 * (documentation/07-api-specification.md §3-4). Kept as plain functions
 * rather than a Zustand store — this is server state, not client/UI state
 * (documentation/08-frontend-architecture.md §1's "Zustand — auth/UI only");
 * TanStack Query isn't installed yet, so pages call these directly and hold
 * the result in local state, same pattern as the rest of Phase 2/3.
 */

import { apiClient } from './apiClient'

export interface Repository {
  id: string
  provider: string
  url: string
  owner: string
  name: string
  default_branch: string
  is_private: boolean
  size_kb: number | null
}

export interface Project {
  id: string
  name: string
  description: string | null
  status: 'active' | 'archived'
  repository: Repository | null
  has_credential: boolean
  latest_scan: unknown
  created_at: string
}

export interface Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface ProjectList {
  data: Project[]
  pagination: Pagination
}

export interface CredentialInfo {
  has_credential: boolean
  hint: string
  updated_at: string | null
}

export interface Target {
  id: string
  target: string
  normalized_host: string
  pinned_ips: string[]
  status: 'awaiting_attestation' | 'attested' | 'blocked' | 'revoked'
  last_resolved_at: string
}

export interface TargetList {
  data: Target[]
}

export function listProjects(): Promise<ProjectList> {
  return apiClient.get<ProjectList>('/projects')
}

export function getProject(id: string): Promise<Project> {
  return apiClient.get<Project>(`/projects/${id}`)
}

export function createProject(input: {
  name: string
  description?: string
  repository_url?: string
}): Promise<Project> {
  return apiClient.post<Project>('/projects', input)
}

export function updateProject(
  id: string,
  input: { name?: string; description?: string },
): Promise<Project> {
  return apiClient.patch<Project>(`/projects/${id}`, input)
}

/** `project.Service.Archive` — backend-ready since Phase 3, first UI use is
 * `ContextMenu`'s "Archive project" (documentation/09-ui-ux-design-system.md
 * §4.4). */
export function archiveProject(id: string): Promise<void> {
  return apiClient.delete<void>(`/projects/${id}`)
}

export function attachRepository(projectId: string, repositoryUrl: string): Promise<Repository> {
  return apiClient.post<Repository>(`/projects/${projectId}/repository`, {
    repository_url: repositoryUrl,
  })
}

export function setCredential(projectId: string, token: string): Promise<CredentialInfo> {
  return apiClient.put<CredentialInfo>(`/projects/${projectId}/credential`, {
    kind: 'github_pat',
    token,
  })
}

export function listTargets(projectId: string): Promise<TargetList> {
  return apiClient.get<TargetList>(`/projects/${projectId}/targets`)
}

export function registerTarget(projectId: string, target: string): Promise<Target> {
  return apiClient.post<Target>(`/projects/${projectId}/targets`, { target })
}
