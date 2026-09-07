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
  /** True once a scan's clone was rejected (401/403) with the stored GitHub
   * PAT — the project and every past scan/finding are unaffected; this is
   * cleared the next time a credential is (re)attached (RepositoryAttachForm). */
  credential_invalid: boolean
  credential_invalid_reason: string | null
}

export interface Project {
  id: string
  name: string
  description: string | null
  status: 'active' | 'archived'
  repository: Repository | null
  has_credential: boolean
  /** Whether this project has an attested pentest target — a cheap summary
   * flag, not the target itself (targets stay their own resource via
   * `listTargets`/`GET /projects/{id}/targets`). Drives which engines
   * `lib/engines.ts`'s `isEngineRunnable` allows selecting. Optional because
   * older/cached responses may not carry it yet — treated as `false` (no
   * target) wherever it's missing. */
  has_pentest_target?: boolean
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

/** `documentation/07-api-specification.md` §4's example statement — the
 * backend validates this is non-empty but doesn't store it verbatim (only
 * `attestation_text_version` is persisted), so this is display copy, not a
 * value the server checks character-for-character. */
export const ATTESTATION_STATEMENT =
  'I confirm I own or am explicitly authorised to test this target.'
export const ATTESTATION_TEXT_VERSION = 'v1'

export interface AttestedBy {
  id: string
  display_name: string
}

export interface TargetAttestation {
  id: string
  status: Target['status']
  attested_at: string
  attested_by: AttestedBy
}

/** A document uploaded for `docreview`'s AI architecture/security review
 * (Phase 11) — content itself is never returned by the API, only this
 * metadata. */
export interface Document {
  id: string
  filename: string
  mime_type: string
  size_bytes: number
  uploaded_by: string | null
  created_at: string
}

export interface DocumentList {
  data: Document[]
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

/** `POST /targets/{id}/attest` (documentation/07-api-specification.md §4) —
 * note the path is target-scoped, not nested under the project. Required
 * before any pentest scan can start (NFR-CMP-001); `target.not_attested`
 * (409) otherwise. */
export function attestTarget(targetId: string): Promise<TargetAttestation> {
  return apiClient.post<TargetAttestation>(`/targets/${targetId}/attest`, {
    attestation_text_version: ATTESTATION_TEXT_VERSION,
    accepted: true,
    statement: ATTESTATION_STATEMENT,
  })
}

export function listDocuments(projectId: string): Promise<DocumentList> {
  return apiClient.get<DocumentList>(`/projects/${projectId}/documents`)
}

export function uploadDocument(projectId: string, file: File): Promise<Document> {
  const form = new FormData()
  form.append('file', file)
  return apiClient.postForm<Document>(`/projects/${projectId}/documents`, form)
}

/** `project.Service.ImportDocumentFromURL` — the "paste a link" counterpart
 * to `uploadDocument`: the backend fetches a Google Docs/Drive share link
 * itself (must be shared "anyone with the link can view"), no OAuth or
 * Google Cloud credentials involved on either side. */
export function importDocument(projectId: string, url: string): Promise<Document> {
  return apiClient.post<Document>(`/projects/${projectId}/documents/import`, { url })
}

export function deleteDocument(documentId: string): Promise<void> {
  return apiClient.delete<void>(`/documents/${documentId}`)
}

// --- project assignments — BUILD_GUIDE.md Phase 15 (the Team Dashboard's
// underlying data) ---

export interface ProjectAssignment {
  project_id: string
  user_id: string
  assigned_by: string | null
  assigned_at: string
}

export function listAssignments(projectId: string): Promise<{ data: ProjectAssignment[] }> {
  return apiClient.get(`/projects/${projectId}/assignments`)
}

export function assignProject(projectId: string, userId: string): Promise<ProjectAssignment> {
  return apiClient.post(`/projects/${projectId}/assignments`, { user_id: userId })
}

export function unassignProject(projectId: string, userId: string): Promise<void> {
  return apiClient.delete(`/projects/${projectId}/assignments/${encodeURIComponent(userId)}`)
}

/** Every assignment across every one of the caller's org's projects, for
 * the Team Dashboard's matrix — client-composed with
 * `organizationApi.listMembers` and each project's latest scan, the same
 * "client-side composition of existing endpoints" precedent
 * `GlobalDashboardPage` already established for Phase 13. */
export function listAssignmentsForOrg(): Promise<{ data: ProjectAssignment[] }> {
  return apiClient.get('/team/assignments')
}

// --- project collaborators (project-collaborators follow-up) — unlike
// assignments above, these actually grant access to someone who need not
// already be a member of this project's org. Mirrors organizationApi.ts's
// own invite/accept/decline/switch shape almost exactly, project-scoped
// instead of org-scoped.

export type ProjectRole = 'admin' | 'member' | 'viewer'

export interface ProjectInvite {
  id: string
  project_id: string
  email: string
  role: ProjectRole
  status: 'pending' | 'accepted' | 'expired' | 'revoked' | 'declined'
  expires_at: string
  created_at: string
}

export interface CreatedProjectInvite extends ProjectInvite {
  /** The raw invite token, shown exactly this once. */
  token: string
}

/** One row of `GET /project-invites/mine` — the live, in-app notification
 * feed, mirroring organizationApi.PendingInvite exactly. */
export interface PendingProjectInvite {
  id: string
  project_id: string
  project_name: string
  org_name: string
  role: ProjectRole
  expires_at: string
  created_at: string
}

export interface ProjectCollaborator {
  project_id: string
  user_id: string
  role: ProjectRole
  created_at: string
}

/** One entry in the "shared projects" switcher list
 * (`GET /collaborations/mine`) — every project (any org) the caller holds
 * an accepted collaborator grant on. */
export interface ProjectCollaboratorSummary {
  project_id: string
  project_name: string
  org_id: string
  org_name: string
  role: ProjectRole
}

export function listCollaboratorInvites(projectId: string): Promise<{ data: ProjectInvite[] }> {
  return apiClient.get(`/projects/${projectId}/collaborators/invites`)
}

export function inviteCollaborator(
  projectId: string,
  email: string,
  role: ProjectRole,
): Promise<CreatedProjectInvite> {
  return apiClient.post(`/projects/${projectId}/collaborators/invites`, { email, role })
}

export function revokeCollaboratorInvite(projectId: string, inviteId: string): Promise<void> {
  return apiClient.delete(
    `/projects/${projectId}/collaborators/invites/${encodeURIComponent(inviteId)}`,
  )
}

export function listCollaborators(projectId: string): Promise<{ data: ProjectCollaborator[] }> {
  return apiClient.get(`/projects/${projectId}/collaborators`)
}

export function removeCollaborator(projectId: string, userId: string): Promise<void> {
  return apiClient.delete(`/projects/${projectId}/collaborators/${encodeURIComponent(userId)}`)
}

/** The live notification feed's own read — NotificationPanel.tsx polls
 * this alongside organizationApi.listMyInvites. */
export function listMyProjectInvites(): Promise<{ data: PendingProjectInvite[] }> {
  return apiClient.get('/project-invites/mine')
}

export function acceptProjectInvite(identifier: string): Promise<ProjectCollaborator> {
  return apiClient.post(`/project-invites/${encodeURIComponent(identifier)}/accept`)
}

export function declineProjectInvite(inviteId: string): Promise<void> {
  return apiClient.post(`/project-invites/${encodeURIComponent(inviteId)}/decline`)
}

/** Every project (any org) the caller holds an accepted collaborator grant
 * on — the "shared projects" switcher's own read. */
export function listMyCollaborations(): Promise<{ data: ProjectCollaboratorSummary[] }> {
  return apiClient.get('/collaborations/mine')
}

/** The shareable accept link for a freshly created project invite's raw
 * token — mirrors organizationApi.inviteAcceptPath exactly, routed to
 * AcceptProjectInvitePage (App.tsx). */
export function projectInviteAcceptPath(token: string): string {
  return `/project-invites/${token}/accept`
}
