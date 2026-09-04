/**
 * Typed wrappers around apiClient for BUILD_GUIDE.md Phase 15's org
 * membership slice — members, invites, switch-org, the org-switcher list.
 * Not yet in documentation/07-api-specification.md, see
 * internal/transport/http/dto/organization.go's own doc-debt note.
 */

import { apiClient } from './apiClient'

export type OrgRole = 'admin' | 'member' | 'viewer'

export interface MemberSummary {
  user_id: string
  email: string
  display_name: string
  role: OrgRole
  is_home: boolean
  joined_at: string | null
}

export interface Invite {
  id: string
  email: string
  role: OrgRole
  status: 'pending' | 'accepted' | 'expired' | 'revoked' | 'declined'
  expires_at: string
  created_at: string
}

/** One row of `GET /invites/mine` — the live, in-app notification feed
 * (2026-09-02 follow-up to Phase 15): an already-registered invitee sees
 * this directly, no link to copy/paste. */
export interface PendingInvite {
  id: string
  org_id: string
  org_name: string
  role: OrgRole
  expires_at: string
  created_at: string
}

export interface CreatedInvite extends Invite {
  /** The raw invite token/link, shown exactly this once — never
   * retrievable again (no email delivery is wired up in this build; share
   * it out of band). */
  token: string
}

export interface MemberOrg {
  org_id: string
  name: string
  role: OrgRole
  is_home: boolean
}

export function listMembers(orgId: string): Promise<{ data: MemberSummary[] }> {
  return apiClient.get(`/organizations/${encodeURIComponent(orgId)}/members`)
}

export function updateMemberRole(
  orgId: string,
  userId: string,
  role: OrgRole,
): Promise<{ data: MemberSummary[] }> {
  return apiClient.patch(
    `/organizations/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}`,
    { role },
  )
}

export function removeMember(orgId: string, userId: string): Promise<void> {
  return apiClient.delete(
    `/organizations/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}`,
  )
}

export function listInvites(orgId: string): Promise<{ data: Invite[] }> {
  return apiClient.get(`/organizations/${encodeURIComponent(orgId)}/invites`)
}

export function createInvite(orgId: string, email: string, role: OrgRole): Promise<CreatedInvite> {
  return apiClient.post(`/organizations/${encodeURIComponent(orgId)}/invites`, { email, role })
}

export function revokeInvite(orgId: string, inviteId: string): Promise<void> {
  return apiClient.delete(
    `/organizations/${encodeURIComponent(orgId)}/invites/${encodeURIComponent(inviteId)}`,
  )
}

/** identifier is either a pending invite's id (the live-notification flow,
 * NotificationPanel.tsx — the primary one) or its raw token (the
 * copy-a-link fallback, AcceptInvitePage.tsx, for someone not logged in
 * yet when the invite arrives) — the backend accepts either
 * (organization.Service.AcceptInvite's own doc comment). */
export function acceptInvite(identifier: string): Promise<{ org_id: string; role: OrgRole }> {
  return apiClient.post(`/invites/${encodeURIComponent(identifier)}/accept`)
}

export function declineInvite(inviteId: string): Promise<void> {
  return apiClient.post(`/invites/${encodeURIComponent(inviteId)}/decline`)
}

/** The live notification feed's own read — NotificationPanel.tsx polls
 * this. */
export function listMyInvites(): Promise<{ data: PendingInvite[] }> {
  return apiClient.get('/invites/mine')
}

export function listMemberOrgs(): Promise<{ data: MemberOrg[] }> {
  return apiClient.get('/organizations')
}

/** The shareable accept link for a freshly created invite's raw token —
 * routed to AcceptInvitePage (App.tsx), which handles both "already logged
 * in as the invited account" (accepts immediately) and "not logged in yet"
 * (send them to /login or /register with the token preserved) — see that
 * page's own doc comment. */
export function inviteAcceptPath(token: string): string {
  return `/invites/${token}/accept`
}
