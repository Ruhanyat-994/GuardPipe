import { useEffect, useState } from 'react'
import { UserPlus, Users, X } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { ApiError } from '../../lib/apiClient'
import {
  assignProject,
  listAssignments,
  unassignProject,
  type ProjectAssignment,
} from '../../lib/projectsApi'
import { listMembers, type MemberSummary } from '../../lib/organizationApi'
import { useAuthStore } from '../../stores/authStore'

/**
 * Per-project teammate assignment (BUILD_GUIDE.md Phase 15) — feeds the
 * Team Dashboard's own matrix; assigning here grants no extra access, it's
 * a workload/visibility label (project.ProjectAssignment's own doc
 * comment), not an RBAC change.
 */
export function ProjectAssignmentsSection({ projectId }: { projectId: string }) {
  const orgId = useAuthStore((s) => s.user?.orgId ?? '')
  const [assignments, setAssignments] = useState<ProjectAssignment[] | null>(null)
  const [members, setMembers] = useState<MemberSummary[]>([])
  const [selected, setSelected] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  function refresh() {
    listAssignments(projectId)
      .then((res) => setAssignments(res.data))
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.problem.detail : 'Could not load assignments.')
      })
  }

  useEffect(refresh, [projectId])
  useEffect(() => {
    if (!orgId) return
    listMembers(orgId)
      .then((res) => setMembers(res.data))
      .catch(() => setMembers([]))
  }, [orgId])

  async function handleAssign() {
    if (!selected) return
    setBusy(selected)
    setError(null)
    try {
      await assignProject(projectId, selected)
      setSelected('')
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not assign this teammate.')
    } finally {
      setBusy(null)
    }
  }

  async function handleUnassign(userId: string) {
    setBusy(userId)
    try {
      await unassignProject(projectId, userId)
      refresh()
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not remove this assignment.')
    } finally {
      setBusy(null)
    }
  }

  const assignedIds = new Set((assignments ?? []).map((a) => a.user_id))
  const memberById = new Map(members.map((m) => [m.user_id, m]))
  const availableMembers = members.filter((m) => !assignedIds.has(m.user_id))

  return (
    <Card className="mb-4">
      <CardTitle className="flex items-center gap-2 text-h3">
        <Users className="h-4 w-4" aria-hidden="true" />
        Assigned teammates
      </CardTitle>
      <CardDescription className="mt-1">
        Shows up on the Team Dashboard — doesn&rsquo;t change anyone&rsquo;s access to this project.
      </CardDescription>

      {error && (
        <p role="alert" className="mt-3 text-body-sm text-danger">
          {error}
        </p>
      )}

      <ul className="mt-3 flex flex-wrap gap-2">
        {(assignments ?? []).map((a) => {
          const m = memberById.get(a.user_id)
          return (
            <li
              key={a.user_id}
              className="flex items-center gap-1.5 rounded-full border border-border-default bg-bg-subtle py-1 pl-3 pr-1.5 text-body-sm text-text-primary"
            >
              {m?.display_name ?? a.user_id}
              <button
                type="button"
                aria-label={`Remove ${m?.display_name ?? 'teammate'}`}
                disabled={busy === a.user_id}
                onClick={() => void handleUnassign(a.user_id)}
                className="rounded-full p-0.5 hover:bg-bg-surface"
              >
                <X className="h-3 w-3" aria-hidden="true" />
              </button>
            </li>
          )
        })}
        {assignments?.length === 0 && (
          <li className="text-body-sm text-text-tertiary">No teammates assigned yet.</li>
        )}
      </ul>

      {availableMembers.length > 0 && (
        <div className="mt-3 flex items-center gap-2">
          <select
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
            className="h-9 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary"
          >
            <option value="">Choose a teammate…</option>
            {availableMembers.map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.display_name}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            loading={busy === selected}
            disabled={!selected}
            onClick={() => void handleAssign()}
          >
            <UserPlus className="h-3.5 w-3.5" aria-hidden="true" />
            Assign
          </Button>
        </div>
      )}
    </Card>
  )
}
