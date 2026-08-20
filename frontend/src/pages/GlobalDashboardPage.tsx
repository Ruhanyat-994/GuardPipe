import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, LayoutGrid } from 'lucide-react'
import { Cell, Pie, PieChart, ResponsiveContainer } from 'recharts'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { RiskGauge } from '../components/dashboard/RiskGauge'
import {
  SeverityStatTile,
  type Severity as TileSeverity,
} from '../components/dashboard/SeverityStatTile'
import { ApiError } from '../lib/apiClient'
import { relativeTime } from '../lib/format'
import { listProjects, type Project } from '../lib/projectsApi'
import { computeRiskScore, sumCounts } from '../lib/riskScore'
import {
  listFindings,
  listOrgScans,
  type FindingListItem,
  type OrgScanSummary,
} from '../lib/scansApi'
import { useAuthStore } from '../stores/authStore'

const SEVERITY_TILES: TileSeverity[] = ['critical', 'high', 'medium', 'low', 'info']
const DONUT_COLORS: Record<TileSeverity, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  info: 'var(--sev-info)',
}

// backend keys are "informational"; the tile/donut components use "info".
const SEVERITY_QUERY: Record<TileSeverity, string> = {
  critical: 'critical',
  high: 'high',
  medium: 'medium',
  low: 'low',
  info: 'informational',
}

function toTileCounts(counts: Record<string, number>): Record<TileSeverity, number> {
  return {
    critical: counts.critical ?? 0,
    high: counts.high ?? 0,
    medium: counts.medium ?? 0,
    low: counts.low ?? 0,
    info: counts.informational ?? 0,
  }
}

interface ProjectRow {
  project: Project
  latestScan: OrgScanSummary | null
}

/**
 * `/dashboard` — the org-wide landing page after login (replaces landing
 * directly on `/projects`). One real data source, one call:
 * `GET /scans` already returns every scan across every project with its
 * finding_counts and project_name attached (built for GlobalScansPage,
 * reused here) — no per-project N+1 needed for the aggregate numbers or the
 * projects table. Only "Top findings" needs finding-level detail, and only
 * for the handful of scans that actually have severe findings, so that's
 * the one place this fans out to a few extra requests, deliberately capped.
 *
 * Deliberately does not repeat the per-project Supply Chain status here —
 * that's inherently a single-scan concept and lives on the project
 * dashboard instead (DashboardPage.tsx), so the two pages complement each
 * other instead of duplicating the same widget at two scopes.
 */
export function GlobalDashboardPage() {
  const user = useAuthStore((s) => s.user)
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [scans, setScans] = useState<OrgScanSummary[] | null>(null)
  const [topFindings, setTopFindings] = useState<
    (FindingListItem & { projectName: string; projectId: string })[] | null
  >(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    Promise.all([listProjects(), listOrgScans(1, 100)])
      .then(async ([projectsRes, scansRes]) => {
        if (cancelled) return
        setProjects(projectsRes.data)
        setScans(scansRes.data)

        // Latest scan per project (scansRes is already newest-first) with the
        // worst finding_counts — the handful most worth surfacing individual
        // findings for. Capped at 3 scans so this stays a few requests, not
        // one per project.
        const latestByProject = new Map<string, OrgScanSummary>()
        for (const s of scansRes.data) {
          if (!latestByProject.has(s.project_id)) latestByProject.set(s.project_id, s)
        }
        const worstFirst = [...latestByProject.values()].sort((a, b) => {
          const score = (c: Record<string, number>) =>
            (c.critical ?? 0) * 1000 + (c.high ?? 0) * 100 + (c.medium ?? 0) * 10 + (c.low ?? 0)
          return score(b.finding_counts) - score(a.finding_counts)
        })
        const candidates = worstFirst.filter((s) => s.status === 'completed').slice(0, 3)

        const findingsPerScan = await Promise.all(
          candidates.map((s) =>
            listFindings(s.id)
              .then((res) =>
                res.data.map((f) => ({
                  ...f,
                  projectName: s.project_name,
                  projectId: s.project_id,
                })),
              )
              .catch(() => []),
          ),
        )
        if (cancelled) return
        const severityRank: Record<string, number> = {
          critical: 0,
          high: 1,
          medium: 2,
          low: 3,
          informational: 4,
        }
        const flattened = findingsPerScan
          .flat()
          .sort((a, b) => severityRank[a.severity] - severityRank[b.severity])
          .slice(0, 5)
        setTopFindings(flattened)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load the dashboard.')
      })

    return () => {
      cancelled = true
    }
  }, [])

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

  if (projects === null || scans === null) {
    return (
      <main className="mx-auto max-w-[1440px] px-6 py-8">
        <p className="text-body-sm text-text-secondary">Loading dashboard…</p>
      </main>
    )
  }

  const rows: ProjectRow[] = projects.map((project) => ({
    project,
    latestScan: scans.find((s) => s.project_id === project.id) ?? null,
  }))

  const completedLatestCounts = rows
    .filter((r) => r.latestScan?.status === 'completed')
    .map((r) => r.latestScan!.finding_counts)
  const totals = sumCounts(completedLatestCounts)
  const { score, verdict } = computeRiskScore(totals)
  const tileCounts = toTileCounts(totals)
  const donutData = SEVERITY_TILES.map((sev) => ({ severity: sev, count: tileCounts[sev] }))
  const hasAnyFindings = Object.values(tileCounts).some((c) => c > 0)

  return (
    <main className="mx-auto max-w-[1440px] px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Welcome back, {user?.displayName ?? ''}</h1>
        <p className="text-body-sm text-text-secondary">
          Across {projects.length} project{projects.length === 1 ? '' : 's'} — showing each
          project's most recent completed scan.
        </p>
      </div>

      {projects.length === 0 ? (
        <Card className="flex flex-col items-center gap-3 py-16 text-center">
          <LayoutGrid className="h-10 w-10 text-text-tertiary" aria-hidden="true" />
          <CardTitle>No projects yet</CardTitle>
          <CardDescription className="max-w-sm">
            Create a project and run its first scan to see your organisation's risk picture here.
          </CardDescription>
          <Link
            to="/projects/new"
            className="mt-1 inline-flex items-center gap-1 text-body-sm font-medium text-accent hover:underline"
          >
            New Project
            <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
          </Link>
        </Card>
      ) : !hasAnyFindings ? (
        <Card className="mb-6">
          <CardDescription>
            No completed scans yet. Run a scan from any project to populate this dashboard.
          </CardDescription>
        </Card>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-[auto_1fr]">
            <Card className="flex flex-col items-center justify-center gap-2">
              <RiskGauge score={score} verdict={verdict} />
              <p className="max-w-[160px] text-center text-caption text-text-tertiary">
                Provisional, severity-weighted — full risk scoring lands with Phase 13.
              </p>
            </Card>

            <Card>
              <CardTitle>Findings across your projects</CardTitle>
              <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-5">
                {SEVERITY_TILES.map((sev) => (
                  <Link
                    key={sev}
                    to={`/findings?severity=${SEVERITY_QUERY[sev]}`}
                    className="rounded-lg transition-transform hover:-translate-y-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2"
                  >
                    <SeverityStatTile severity={sev} count={tileCounts[sev]} />
                  </Link>
                ))}
              </div>

              <div className="mt-6 flex flex-col items-center gap-4 sm:flex-row">
                <div className="h-40 w-40 shrink-0">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={donutData}
                        dataKey="count"
                        nameKey="severity"
                        innerRadius={40}
                        outerRadius={70}
                        paddingAngle={2}
                      >
                        {donutData.map((d) => (
                          <Cell key={d.severity} fill={DONUT_COLORS[d.severity]} stroke="none" />
                        ))}
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                </div>
                <ul className="flex flex-col gap-1.5 text-body-sm">
                  {SEVERITY_TILES.map((sev) => (
                    <li key={sev} className="flex items-center gap-2">
                      <span
                        className="h-2.5 w-2.5 rounded-full"
                        style={{ backgroundColor: DONUT_COLORS[sev] }}
                        aria-hidden="true"
                      />
                      <span className="capitalize text-text-secondary">{sev}</span>
                      <span className="font-medium text-text-primary">{tileCounts[sev]}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </Card>
          </div>

          {topFindings !== null && topFindings.length > 0 && (
            <Card className="mt-6">
              <CardTitle>Top findings</CardTitle>
              <ul className="mt-4 flex flex-col divide-y divide-border-default">
                {topFindings.map((f) => (
                  <li key={f.id} className="flex items-center gap-3 py-3">
                    <span
                      className="rounded-full px-2 py-0.5 text-caption font-semibold text-white uppercase"
                      style={{ backgroundColor: `var(--sev-${f.severity})` }}
                    >
                      {f.severity}
                    </span>
                    <span className="flex-1 truncate text-body-sm text-text-primary">
                      {f.title}
                    </span>
                    <Link
                      to={`/projects/${f.projectId}`}
                      className="text-caption text-text-tertiary hover:text-accent hover:underline"
                    >
                      {f.projectName}
                    </Link>
                  </li>
                ))}
              </ul>
            </Card>
          )}
        </>
      )}

      <Card className="mt-6">
        <div className="flex items-center justify-between">
          <CardTitle>Projects</CardTitle>
          <Link
            to="/projects"
            className="inline-flex items-center gap-1 text-caption font-medium text-accent hover:underline"
          >
            View all
            <ArrowRight className="h-3 w-3" aria-hidden="true" />
          </Link>
        </div>
        <ul className="mt-4 flex flex-col divide-y divide-border-default">
          {rows.map(({ project, latestScan }) => {
            const counts = toTileCounts(latestScan?.finding_counts ?? {})
            return (
              <li key={project.id}>
                <Link
                  to={`/projects/${project.id}`}
                  className="flex items-center gap-4 py-3 hover:bg-bg-subtle"
                >
                  <span className="min-w-0 flex-1 truncate text-body-sm font-medium text-text-primary">
                    {project.name}
                  </span>
                  {latestScan ? (
                    <span className="hidden shrink-0 gap-1.5 sm:flex">
                      {SEVERITY_TILES.filter((sev) => counts[sev] > 0).map((sev) => (
                        <span
                          key={sev}
                          className="rounded-full px-1.5 py-0.5 text-caption font-semibold text-white"
                          style={{ backgroundColor: DONUT_COLORS[sev] }}
                        >
                          {counts[sev]}
                        </span>
                      ))}
                    </span>
                  ) : (
                    <span className="shrink-0 text-caption text-text-tertiary">No scans yet</span>
                  )}
                  <span className="w-24 shrink-0 text-right text-caption text-text-tertiary">
                    {latestScan ? relativeTime(latestScan.queued_at) : ''}
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      </Card>
    </main>
  )
}
