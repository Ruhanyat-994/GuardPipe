import { useEffect, useState } from 'react'
import { Copy, Mail, UserPlus, Users2, X } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { Input } from '../ui/Input'
import { ApiError } from '../../lib/apiClient'
import { formatDate } from '../../lib/format'
import {
  inviteCollaborator,
  listCollaboratorInvites,
  listCollaborators,
  projectInviteAcceptPath,
  removeCollaborator,
  revokeCollaboratorInvite,
  type CreatedProjectInvite,
  type ProjectCollaborator,
  type ProjectInvite,
  type ProjectRole,
} from '../../lib/projectsApi'

const ROLE_OPTIONS: { value: ProjectRole; label: string }[] = [
  { value: 'admin', label: 'Admin' },
  { value: 'member', label: 'Member' },
  { value: 'viewer', label: 'Viewer' },
]

/**
 * project-collaborators follow-up — invite a GuardPipe user (by email, need
 * not already be a member of this org) to just this one project, with a
 * role the inviter picks. Unlike ProjectAssignmentsSection, accepting one of
 * these invites is a real access grant (project.ProjectCollaborator's own
 * doc comment): the invitee gets nothing beyond this one project, never the
 * rest of this org's dashboard, and only once they've explicitly accepted.
 */
export function ProjectCollaboratorsSection({ projectId }: { projectId: string }) {
  const [collaborators, setCollaborators] = useState<ProjectCollaborator[] | null>(null)
  const [invites, setInvites] = useState<ProjectInvite[] | null>(null)
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<ProjectRole>('viewer')
  const [inviting, setInviting] = useState(false)
  const [busyKey, setBusyKey] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [inviteResult, setInviteResult] = useState<CreatedProjectInvite | null>(null)

  function refresh() {
    listCollaborators(projectId)
      .then((res) => setCollaborators(res.data))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load collaborators.')
      })
    listCollaboratorInvites(projectId)
      .then((res) => setInvites(res.data.filter((i) => i.status === 'pending')))
      .catch(() => setInvites([]))
  }

  useEffect(refresh, [projectId])

  async function handleInvite() {
    const trimmed = email.trim()
    if (!trimmed) return
    setInviting(true)
    setError(null)
    try {
      const created = await inviteCollaborator(projectId, trimmed, role)
      setInviteResult(created)
      setEmail('')
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not send this invite.')
    } finally {
      setInviting(false)
    }
  }

  async function handleRevoke(inviteId: string) {
    setBusyKey(inviteId)
    try {
      await revokeCollaboratorInvite(projectId, inviteId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not revoke this invite.')
    } finally {
      setBusyKey(null)
    }
  }

  async function handleRemove(userId: string) {
    setBusyKey(userId)
    try {
      await removeCollaborator(projectId, userId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not remove this collaborator.')
    } finally {
      setBusyKey(null)
    }
  }

  const acceptUrl = inviteResult
    ? `${window.location.origin}${projectInviteAcceptPath(inviteResult.token)}`
    : ''

  return (
    <Card className="mb-4">
      <CardTitle className="flex items-center gap-2 text-h3">
        <Users2 className="h-4 w-4" aria-hidden="true" />
        Project collaborators
      </CardTitle>
      <CardDescription className="mt-1">
        Give a GuardPipe user access to just this project — they don&rsquo;t need to already be in
        your organization, and they only see this one project, never the rest of your dashboard,
        until they explicitly accept.
      </CardDescription>

      {error && (
        <p role="alert" className="mt-3 text-body-sm text-danger">
          {error}
        </p>
      )}

      <ul className="mt-3 flex flex-wrap gap-2">
        {(collaborators ?? []).map((c) => (
          <li
            key={c.user_id}
            className="flex items-center gap-1.5 rounded-full border border-border-default bg-bg-subtle py-1 pl-3 pr-1.5 text-body-sm text-text-primary"
          >
            <span className="capitalize text-text-tertiary">{c.role}</span>
            <span className="truncate">{c.user_id}</span>
            <button
              type="button"
              aria-label="Remove collaborator"
              disabled={busyKey === c.user_id}
              onClick={() => void handleRemove(c.user_id)}
              className="rounded-full p-0.5 hover:bg-bg-surface"
            >
              <X className="h-3 w-3" aria-hidden="true" />
            </button>
          </li>
        ))}
        {collaborators?.length === 0 && (
          <li className="text-body-sm text-text-tertiary">No external collaborators yet.</li>
        )}
      </ul>

      {invites && invites.length > 0 && (
        <ul className="mt-2 flex flex-col gap-1.5">
          {invites.map((inv) => (
            <li
              key={inv.id}
              className="flex items-center justify-between gap-2 rounded-md border border-dashed border-border-default px-3 py-1.5 text-body-sm text-text-secondary"
            >
              <span className="flex items-center gap-1.5 truncate">
                <Mail className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                {inv.email} <span className="capitalize text-text-tertiary">({inv.role})</span>{' '}
                <span className="text-caption text-text-tertiary">pending</span>
              </span>
              <Button
                size="sm"
                variant="secondary"
                loading={busyKey === inv.id}
                onClick={() => void handleRevoke(inv.id)}
              >
                Revoke
              </Button>
            </li>
          ))}
        </ul>
      )}

      {inviteResult && (
        <div className="mt-3 rounded-md border border-accent/30 bg-accent/5 p-3">
          <p className="text-body-sm font-medium text-text-primary">
            Invite created for {inviteResult.email}
          </p>
          <p className="mt-1 text-caption text-text-tertiary">
            They&rsquo;ll see it live in their notifications if they already have an account.
            Otherwise, share this link — it works only once, and expires{' '}
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

      <div className="mt-3 flex items-center gap-2">
        <input
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="teammate@example.com"
          className="h-9 min-w-0 flex-1 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary"
        />
        <select
          value={role}
          onChange={(e) => setRole(e.target.value as ProjectRole)}
          className="h-9 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary"
        >
          {ROLE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        <Button
          size="sm"
          loading={inviting}
          disabled={!email.trim()}
          onClick={() => void handleInvite()}
        >
          <UserPlus className="h-3.5 w-3.5" aria-hidden="true" />
          Invite
        </Button>
      </div>
    </Card>
  )
}
