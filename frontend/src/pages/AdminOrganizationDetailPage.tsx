import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, ShieldOff, Users } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { ReasonDialog } from '../components/admin/ReasonDialog'
import { ApiError } from '../lib/apiClient'
import {
  getOrganization,
  reinstateOrganization,
  reinstateUser,
  suspendOrganization,
  suspendUser,
  type OrganizationDetail,
} from '../lib/adminApi'
import { formatDate } from '../lib/format'

type PendingAction =
  | { kind: 'suspend-org' }
  | { kind: 'suspend-user'; userId: string; displayName: string }
  | null

/** Screen 18 detail view — Admin: Organization (BUILD_GUIDE.md Phase 14). */
export function AdminOrganizationDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [org, setOrg] = useState<OrganizationDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [pending, setPending] = useState<PendingAction>(null)

  function refresh() {
    if (!id) return
    getOrganization(id)
      .then(setOrg)
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load this organization.')
      })
  }

  useEffect(refresh, [id])

  async function handleReinstateOrg() {
    if (!id) return
    setBusy(true)
    try {
      await reinstateOrganization(id)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not reinstate this organization.')
    } finally {
      setBusy(false)
    }
  }

  async function handleReinstateUser(userId: string) {
    setBusy(true)
    try {
      await reinstateUser(userId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not reinstate this user.')
    } finally {
      setBusy(false)
    }
  }

  async function handleConfirmReason(reason: string) {
    if (!id || !pending) return
    if (pending.kind === 'suspend-org') {
      await suspendOrganization(id, reason)
    } else {
      await suspendUser(pending.userId, reason)
    }
    setPending(null)
    refresh()
  }

  if (error && !org) {
    return (
      <main className="mx-auto max-w-4xl px-6 py-8">
        <Card className="border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      </main>
    )
  }

  if (!org) {
    return (
      <main className="mx-auto max-w-4xl px-6 py-8">
        <CardDescription>Loading…</CardDescription>
      </main>
    )
  }

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <Link
        to="/admin/organizations"
        className="mb-4 inline-flex items-center gap-1.5 text-body-sm text-text-secondary hover:text-text-primary"
      >
        <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        All organizations
      </Link>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      <Card className="mb-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-h1 text-text-primary">{org.name}</h1>
            <p className="mt-1 text-caption text-text-tertiary">
              Created {formatDate(org.created_at)} · {org.project_count} projects ·{' '}
              {org.scan_count} scans
            </p>
          </div>
          {org.suspended_at ? (
            <Button variant="secondary" loading={busy} onClick={() => void handleReinstateOrg()}>
              Reinstate organization
            </Button>
          ) : (
            <Button
              variant="destructive"
              onClick={() => setPending({ kind: 'suspend-org' })}
            >
              <ShieldOff className="h-4 w-4" aria-hidden="true" />
              Suspend organization
            </Button>
          )}
        </div>

        {org.suspended_at && (
          <div className="mt-4 rounded-md border border-danger/30 bg-danger/5 p-3 text-body-sm text-danger">
            Suspended {formatDate(org.suspended_at)}
            {org.suspended_reason && <> — {org.suspended_reason}</>}
          </div>
        )}
      </Card>

      <Card>
        <CardTitle className="flex items-center gap-2 text-h3">
          <Users className="h-4 w-4" aria-hidden="true" />
          Members ({org.members.length})
        </CardTitle>

        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-left text-body-sm">
            <thead>
              <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                <th className="pb-2 pr-4 font-semibold">Name</th>
                <th className="pb-2 pr-4 font-semibold">Role</th>
                <th className="pb-2 pr-4 font-semibold">Last login</th>
                <th className="pb-2 pr-4 font-semibold">Status</th>
                <th className="pb-2 font-semibold" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border-default">
              {org.members.map((member) => (
                <tr key={member.id}>
                  <td className="py-2.5 pr-4">
                    <div className="font-medium text-text-primary">{member.display_name}</div>
                    <div className="text-caption text-text-tertiary">{member.email}</div>
                  </td>
                  <td className="py-2.5 pr-4 text-text-secondary capitalize">{member.role}</td>
                  <td className="py-2.5 pr-4 text-text-tertiary">
                    {member.last_login_at ? formatDate(member.last_login_at) : 'Never'}
                  </td>
                  <td className="py-2.5 pr-4">
                    {member.suspended_at ? (
                      <span className="inline-flex items-center rounded-full bg-danger/10 px-2 py-0.5 text-caption font-semibold text-danger">
                        Suspended
                      </span>
                    ) : (
                      <span className="inline-flex items-center rounded-full bg-success/10 px-2 py-0.5 text-caption font-semibold text-success">
                        Active
                      </span>
                    )}
                  </td>
                  <td className="py-2.5 text-right">
                    {member.suspended_at ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        loading={busy}
                        onClick={() => void handleReinstateUser(member.id)}
                      >
                        Reinstate
                      </Button>
                    ) : (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() =>
                          setPending({
                            kind: 'suspend-user',
                            userId: member.id,
                            displayName: member.display_name,
                          })
                        }
                      >
                        Suspend
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {pending?.kind === 'suspend-org' && (
        <ReasonDialog
          title={`Suspend ${org.name}?`}
          description="Every member of this organization will be signed out and unable to log back in until reinstated."
          confirmLabel="Suspend organization"
          onConfirm={handleConfirmReason}
          onCancel={() => setPending(null)}
        />
      )}
      {pending?.kind === 'suspend-user' && (
        <ReasonDialog
          title={`Suspend ${pending.displayName}?`}
          description="This person will be signed out and unable to log back in until reinstated. Other members of the organization are unaffected."
          confirmLabel="Suspend user"
          onConfirm={handleConfirmReason}
          onCancel={() => setPending(null)}
        />
      )}
    </main>
  )
}
