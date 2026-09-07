import { useEffect, useState } from 'react'
import { Copy, Mail, Trash2, UserPlus, Users, X } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { ApiError } from '../lib/apiClient'
import {
  createInvite,
  inviteAcceptPath,
  listInvites,
  listMembers,
  removeMember,
  revokeInvite,
  updateMemberRole,
  type CreatedInvite,
  type Invite,
  type MemberSummary,
  type OrgRole,
} from '../lib/organizationApi'
import { useAuthStore } from '../stores/authStore'
import { formatDate } from '../lib/format'

const ROLE_OPTIONS: OrgRole[] = ['admin', 'member', 'viewer']

/**
 * Org Settings → Members (BUILD_GUIDE.md Phase 15) — member list with role
 * badges, an invite form, and the pending-invite list with revoke. Lives at
 * `/settings` (previously a Phase-9 placeholder — this is that phase's real
 * content for the org-membership half of it; per-account profile fields
 * remain a later addition).
 *
 * Every mutating control here is only rendered for an `admin` of the
 * currently active org (role badges/permission-gated controls per
 * BUILD_GUIDE.md Phase 15's frontend checklist) — the server re-checks this
 * independently (organization.Service's own RBAC + requireOwnOrg), this is
 * UX only.
 */
export function OrgSettingsPage() {
  const user = useAuthStore((s) => s.user)
  const orgId = user?.orgId ?? ''
  const isAdmin = user?.role === 'admin'

  const [members, setMembers] = useState<MemberSummary[] | null>(null)
  const [invites, setInvites] = useState<Invite[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<OrgRole>('member')
  const [inviting, setInviting] = useState(false)
  const [inviteResult, setInviteResult] = useState<CreatedInvite | null>(null)

  const [busyUserId, setBusyUserId] = useState<string | null>(null)
  const [busyInviteId, setBusyInviteId] = useState<string | null>(null)

  function refresh() {
    if (!orgId) return
    listMembers(orgId)
      .then((res) => setMembers(res.data))
      .catch((err: unknown) => {
        setError(
          err instanceof ApiError ? err.problem.detail : 'Could not load organization members.',
        )
      })
    if (isAdmin) {
      listInvites(orgId)
        .then((res) => setInvites(res.data))
        .catch(() => setInvites([]))
    }
  }

  useEffect(refresh, [orgId, isAdmin])

  async function handleInvite() {
    if (!orgId || !inviteEmail.trim()) return
    setInviting(true)
    setError(null)
    try {
      const created = await createInvite(orgId, inviteEmail.trim(), inviteRole)
      setInviteResult(created)
      setInviteEmail('')
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not create the invite.')
    } finally {
      setInviting(false)
    }
  }

  async function handleRoleChange(userId: string, role: OrgRole) {
    if (!orgId) return
    setBusyUserId(userId)
    setError(null)
    try {
      await updateMemberRole(orgId, userId, role)
      refresh()
    } catch (err) {
      setError(
        err instanceof ApiError ? err.problem.detail : 'Could not update this member’s role.',
      )
    } finally {
      setBusyUserId(null)
    }
  }

  async function handleRemove(userId: string) {
    if (!orgId) return
    if (!window.confirm('Remove this member from the organization?')) return
    setBusyUserId(userId)
    setError(null)
    try {
      await removeMember(orgId, userId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not remove this member.')
    } finally {
      setBusyUserId(null)
    }
  }

  async function handleRevokeInvite(inviteId: string) {
    if (!orgId) return
    setBusyInviteId(inviteId)
    try {
      await revokeInvite(orgId, inviteId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not revoke this invite.')
    } finally {
      setBusyInviteId(null)
    }
  }

  const acceptUrl = inviteResult
    ? `${window.location.origin}${inviteAcceptPath(inviteResult.token)}`
    : ''

  return (
    <main className="mx-auto max-w-3xl px-6 py-8">
      <h1 className="mb-1 text-h1 text-text-primary">Organization settings</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        Manage who has access to this organization and what role they hold.
      </p>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      {isAdmin && (
        <Card className="mb-4">
          <CardTitle className="flex items-center gap-2 text-h3">
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            Invite a member
          </CardTitle>
          <CardDescription className="mt-1">
            If they already have a GuardPipe account, this shows up live in their notifications — no
            link needed. There&rsquo;s no email delivery in this build yet, so you&rsquo;ll also get
            a link below, only needed if they don&rsquo;t have an account yet.
          </CardDescription>
          <div className="mt-4 flex flex-wrap items-end gap-3">
            <div className="min-w-[220px] flex-1">
              <label htmlFor="invite-email" className="mb-1 block text-body-sm text-text-secondary">
                Email
              </label>
              <Input
                id="invite-email"
                type="email"
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                placeholder="teammate@company.com"
              />
            </div>
            <div>
              <label htmlFor="invite-role" className="mb-1 block text-body-sm text-text-secondary">
                Role
              </label>
              <select
                id="invite-role"
                value={inviteRole}
                onChange={(e) => setInviteRole(e.target.value as OrgRole)}
                className="h-10 rounded-md border border-border-default bg-bg-surface px-3 text-body text-text-primary capitalize"
              >
                {ROLE_OPTIONS.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
            </div>
            <Button
              loading={inviting}
              disabled={!inviteEmail.trim()}
              onClick={() => void handleInvite()}
            >
              <Mail className="h-4 w-4" aria-hidden="true" />
              Send invite
            </Button>
          </div>

          {inviteResult && (
            <div className="mt-4 rounded-md border border-accent/30 bg-accent/5 p-3">
              <p className="text-body-sm font-medium text-text-primary">
                Invite created for {inviteResult.email}
              </p>
              <p className="mt-1 text-caption text-text-tertiary">
                Share this link — it works only once, and expires{' '}
                {formatDate(inviteResult.expires_at)}.
              </p>
              <div className="mt-2 flex items-center gap-2">
                <Input readOnly value={acceptUrl} className="text-caption" />
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => void navigator.clipboard.writeText(acceptUrl)}
                >
                  <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                  Copy
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setInviteResult(null)}>
                  <X className="h-3.5 w-3.5" aria-hidden="true" />
                </Button>
              </div>
            </div>
          )}
        </Card>
      )}

      <Card className="mb-4">
        <CardTitle className="flex items-center gap-2 text-h3">
          <Users className="h-4 w-4" aria-hidden="true" />
          Members {members ? `(${members.length})` : ''}
        </CardTitle>

        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-left text-body-sm">
            <thead>
              <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                <th className="pb-2 pr-4 font-semibold">Name</th>
                <th className="pb-2 pr-4 font-semibold">Role</th>
                <th className="pb-2 pr-4 font-semibold">Joined</th>
                <th className="pb-2 font-semibold" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border-default">
              {(members ?? []).map((m) => (
                <tr key={m.user_id}>
                  <td className="py-2.5 pr-4">
                    <div className="font-medium text-text-primary">{m.display_name}</div>
                    <div className="text-caption text-text-tertiary">{m.email}</div>
                  </td>
                  <td className="py-2.5 pr-4">
                    {isAdmin ? (
                      <select
                        value={m.role}
                        disabled={busyUserId === m.user_id}
                        onChange={(e) =>
                          void handleRoleChange(m.user_id, e.target.value as OrgRole)
                        }
                        className="h-8 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary capitalize"
                      >
                        {ROLE_OPTIONS.map((r) => (
                          <option key={r} value={r}>
                            {r}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <span className="capitalize text-text-secondary">{m.role}</span>
                    )}
                    {m.is_home && (
                      <span className="ml-1.5 text-caption text-text-tertiary">(founder)</span>
                    )}
                  </td>
                  <td className="py-2.5 pr-4 text-text-tertiary">
                    {m.joined_at ? formatDate(m.joined_at) : '—'}
                  </td>
                  <td className="py-2.5 text-right">
                    {isAdmin && !m.is_home && (
                      <Button
                        variant="ghost"
                        size="sm"
                        loading={busyUserId === m.user_id}
                        onClick={() => void handleRemove(m.user_id)}
                      >
                        <Trash2 className="h-3.5 w-3.5 text-danger" aria-hidden="true" />
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
              {members?.length === 0 && (
                <tr>
                  <td colSpan={4} className="py-6 text-center text-text-tertiary">
                    No members yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {isAdmin && invites && invites.length > 0 && (
        <Card>
          <CardTitle className="text-h3">Pending invites</CardTitle>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-left text-body-sm">
              <thead>
                <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                  <th className="pb-2 pr-4 font-semibold">Email</th>
                  <th className="pb-2 pr-4 font-semibold">Role</th>
                  <th className="pb-2 pr-4 font-semibold">Status</th>
                  <th className="pb-2 font-semibold" />
                </tr>
              </thead>
              <tbody className="divide-y divide-border-default">
                {invites.map((inv) => (
                  <tr key={inv.id}>
                    <td className="py-2.5 pr-4 text-text-primary">{inv.email}</td>
                    <td className="py-2.5 pr-4 capitalize text-text-secondary">{inv.role}</td>
                    <td className="py-2.5 pr-4 capitalize text-text-tertiary">{inv.status}</td>
                    <td className="py-2.5 text-right">
                      {inv.status === 'pending' && (
                        <Button
                          variant="ghost"
                          size="sm"
                          loading={busyInviteId === inv.id}
                          onClick={() => void handleRevokeInvite(inv.id)}
                        >
                          <Trash2 className="h-3.5 w-3.5 text-danger" aria-hidden="true" />
                          Revoke
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </main>
  )
}
