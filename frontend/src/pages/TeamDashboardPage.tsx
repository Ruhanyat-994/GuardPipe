import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Users } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { cn } from '../lib/cn'
import { ApiError } from '../lib/apiClient'
import {
  listAssignmentsForOrg,
  listProjects,
  type Project,
  type ProjectAssignment,
} from '../lib/projectsApi'
import { listMembers, type MemberSummary } from '../lib/organizationApi'
import { getScan, listOrgScans, type RiskAssessment } from '../lib/scansApi'
import { useAuthStore } from '../stores/authStore'

interface CellData {
  verdict: RiskAssessment['verdict'] | null
  openHighCount: number
}

const VERDICT_STYLE: Record<RiskAssessment['verdict'], string> = {
  pass: 'bg-success/10 text-success',
  warn: 'bg-warning/10 text-warning',
  block: 'bg-danger/10 text-danger',
}

/**
 * `/team` — the Team Dashboard (BUILD_GUIDE.md Phase 15): a matrix with org
 * members as rows and their assigned projects as columns, each cell driven
 * by that project's latest scan's real gate verdict (modules/scoring,
 * Phase 13's own output — not a parallel "task done" concept) plus an
 * open-critical/high count. Client-side composition of existing endpoints,
 * the same precedent GlobalDashboardPage already set for the org-wide
 * dashboard rather than a dedicated aggregate endpoint.
 */
export function TeamDashboardPage() {
  const orgId = useAuthStore((s) => s.user?.orgId ?? '')
  const [members, setMembers] = useState<MemberSummary[] | null>(null)
  const [assignments, setAssignments] = useState<ProjectAssignment[] | null>(null)
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [cells, setCells] = useState<Map<string, CellData>>(new Map())
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    let cancelled = false

    Promise.all([listMembers(orgId), listAssignmentsForOrg(), listProjects(), listOrgScans(1, 100)])
      .then(async ([membersRes, assignmentsRes, projectsRes, scansRes]) => {
        if (cancelled) return
        setMembers(membersRes.data)
        setAssignments(assignmentsRes.data)
        setProjects(projectsRes.data)

        // Latest completed scan id per assigned project only — bounded fan-out
        // (GlobalDashboardPage's own "top findings" precedent for why this
        // stays a handful of requests, not one per org project).
        const assignedProjectIds = new Set(assignmentsRes.data.map((a) => a.project_id))
        const latestByProject = new Map<string, string>()
        for (const s of scansRes.data) {
          if (
            assignedProjectIds.has(s.project_id) &&
            !latestByProject.has(s.project_id) &&
            s.status === 'completed'
          ) {
            latestByProject.set(s.project_id, s.id)
          }
        }

        const entries = await Promise.all(
          [...latestByProject.entries()].map(async ([projectId, scanId]) => {
            try {
              const scan = await getScan(scanId)
              const openHigh = (scan.finding_counts.critical ?? 0) + (scan.finding_counts.high ?? 0)
              return [
                projectId,
                { verdict: scan.risk?.verdict ?? null, openHighCount: openHigh },
              ] as const
            } catch {
              return [projectId, { verdict: null, openHighCount: 0 }] as const
            }
          }),
        )
        if (cancelled) return
        setCells(new Map(entries))
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(
            err instanceof ApiError ? err.problem.detail : 'Could not load the team dashboard.',
          )
      })

    return () => {
      cancelled = true
    }
  }, [orgId])

  if (error) {
    return (
      <main className="mx-auto max-w-[1440px] px-6 py-8">
        <Card className="border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      </main>
    )
  }

  if (members === null || assignments === null || projects === null) {
    return (
      <main className="mx-auto max-w-[1440px] px-6 py-8">
        <p className="text-body-sm text-text-secondary">Loading team dashboard…</p>
      </main>
    )
  }

  const projectById = new Map(projects.map((p) => [p.id, p]))
  const assignedProjectIds = [...new Set(assignments.map((a) => a.project_id))]
  const columns = assignedProjectIds
    .map((id) => projectById.get(id))
    .filter((p): p is Project => Boolean(p))
  const assignmentSet = new Set(assignments.map((a) => `${a.user_id}|${a.project_id}`))

  return (
    <main className="mx-auto max-w-[1440px] px-6 py-8">
      <div className="mb-6">
        <h1 className="flex items-center gap-2 text-h1 text-text-primary">
          <Users className="h-5 w-5" aria-hidden="true" />
          Team Dashboard
        </h1>
        <p className="text-body-sm text-text-secondary">
          Who&rsquo;s working on what, and whether it&rsquo;s currently passing its gate.
        </p>
      </div>

      {columns.length === 0 ? (
        <Card>
          <CardTitle>No project assignments yet</CardTitle>
          <CardDescription className="mt-1">
            Assign a project to a teammate from that project&rsquo;s settings to see them show up
            here.
          </CardDescription>
        </Card>
      ) : (
        <Card>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-body-sm">
              <thead>
                <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                  <th className="sticky left-0 bg-bg-surface pb-2 pr-4 font-semibold">Member</th>
                  {columns.map((p) => (
                    <th key={p.id} className="pb-2 pr-4 font-semibold">
                      <Link to={`/projects/${p.id}`} className="hover:text-accent hover:underline">
                        {p.name}
                      </Link>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-border-default">
                {members.map((m) => (
                  <tr key={m.user_id}>
                    <td className="sticky left-0 bg-bg-surface py-2.5 pr-4 font-medium text-text-primary">
                      {m.display_name}
                    </td>
                    {columns.map((p) => {
                      const isAssigned = assignmentSet.has(`${m.user_id}|${p.id}`)
                      const cell = cells.get(p.id)
                      if (!isAssigned) {
                        return (
                          <td key={p.id} className="py-2.5 pr-4 text-text-tertiary">
                            —
                          </td>
                        )
                      }
                      return (
                        <td key={p.id} className="py-2.5 pr-4">
                          {cell?.verdict ? (
                            <div className="flex items-center gap-2">
                              <span
                                className={cn(
                                  'inline-flex items-center rounded-full px-2 py-0.5 text-caption font-semibold uppercase',
                                  VERDICT_STYLE[cell.verdict],
                                )}
                              >
                                {cell.verdict}
                              </span>
                              {cell.openHighCount > 0 && (
                                <span className="text-caption text-text-tertiary">
                                  {cell.openHighCount} critical/high
                                </span>
                              )}
                            </div>
                          ) : (
                            <span className="text-caption text-text-tertiary">
                              No completed scan
                            </span>
                          )}
                        </td>
                      )
                    })}
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
