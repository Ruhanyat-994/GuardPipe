import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  CartesianGrid,
  Cell,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { RiskGauge } from '../components/dashboard/RiskGauge'
import {
  SeverityStatTile,
  type Severity as TileSeverity,
} from '../components/dashboard/SeverityStatTile'
import { SupplyChainPipeline } from '../components/project/SupplyChainPipeline'
import { useProjectContext } from '../components/project/ProjectContext'
import { ApiError } from '../lib/apiClient'
import { relativeTime } from '../lib/format'
import { computeRiskScore, sumCounts } from '../lib/riskScore'
import { getScan, listScans, type Scan, type ScanSummary } from '../lib/scansApi'

const SEVERITY_TILES: TileSeverity[] = ['critical', 'high', 'medium', 'low', 'info']
const DONUT_COLORS: Record<TileSeverity, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  info: 'var(--sev-info)',
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

/**
 * `/projects/:id` — this project's own dashboard. Was the hardcoded "Screen
 * 4" preview since Phase 2; now real, using this project's own scan
 * history rather than any org-wide aggregate (that's GlobalDashboardPage,
 * `/dashboard`) — the two pages are deliberately complementary, not
 * duplicates: this one owns Supply Chain status and the risk trend (both
 * inherently scoped to one project's own scans), the global page owns the
 * cross-project table instead.
 */
export function DashboardPage() {
  const { project } = useProjectContext()
  const [recentScans, setRecentScans] = useState<ScanSummary[] | null>(null)
  const [latestScan, setLatestScan] = useState<Scan | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listScans(project.id, 1, 8)
      .then((res) => {
        if (cancelled) return
        setRecentScans(res.data)
        const latest = res.data[0]
        if (latest && latest.status === 'completed') {
          return getScan(latest.id).then((full) => {
            if (!cancelled) setLatestScan(full)
          })
        }
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(
            err instanceof ApiError ? err.problem.detail : 'Could not load this project’s scans.',
          )
      })
    return () => {
      cancelled = true
    }
  }, [project.id])

  if (error) {
    return (
      <main className="mx-auto max-w-6xl px-6 py-8">
        <Card className="border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      </main>
    )
  }

  const header = (
    <div className="mb-6 flex items-center justify-between">
      <div>
        <h1 className="text-h1 text-text-primary">{project.name}</h1>
        <p className="text-body-sm text-text-secondary">
          {project.repository
            ? project.repository.url.replace('https://', '')
            : 'No repository attached'}
          {project.repository?.default_branch ? ` · ${project.repository.default_branch}` : ''}
          {recentScans && recentScans[0]
            ? ` · last scan ${relativeTime(recentScans[0].queued_at)}`
            : ''}
        </p>
      </div>
      <Link to={`/projects/${project.id}/scans`}>
        <Button>Run Scan</Button>
      </Link>
    </div>
  )

  if (recentScans === null) {
    return (
      <main className="mx-auto max-w-6xl px-6 py-8">
        {header}
        <p className="text-body-sm text-text-secondary">Loading…</p>
      </main>
    )
  }

  if (recentScans.length === 0) {
    return (
      <main className="mx-auto max-w-6xl px-6 py-8">
        {header}
        <Card>
          <CardDescription>
            No scans yet — click "Run Scan" to get this project's first risk picture.
          </CardDescription>
        </Card>
      </main>
    )
  }

  const totals = sumCounts(
    recentScans[0].status === 'completed' ? [recentScans[0].finding_counts] : [],
  )
  const { score, verdict } = computeRiskScore(totals)
  const tileCounts = toTileCounts(totals)
  const donutData = SEVERITY_TILES.map((sev) => ({ severity: sev, count: tileCounts[sev] }))

  const trend = [...recentScans]
    .filter((s) => s.status === 'completed')
    .reverse()
    .map((s, i) => ({
      scan: String(i + 1),
      score: computeRiskScore(s.finding_counts).score,
    }))

  return (
    <main className="mx-auto max-w-6xl px-6 py-8">
      {header}

      {recentScans[0].status !== 'completed' ? (
        <Card>
          <CardDescription>
            The most recent scan is {recentScans[0].status} — this dashboard updates once it
            completes.{' '}
            <Link to={`/scans/${recentScans[0].id}`} className="text-accent hover:underline">
              Watch it live
            </Link>
            .
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
              <CardTitle>Current findings</CardTitle>
              <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-5">
                {SEVERITY_TILES.map((sev) => (
                  <SeverityStatTile key={sev} severity={sev} count={tileCounts[sev]} />
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

          {latestScan && (
            <Card className="mt-6">
              <CardTitle>Supply chain</CardTitle>
              <div className="mt-4">
                <SupplyChainPipeline progress={null} jobs={latestScan.jobs} scan={latestScan} />
              </div>
            </Card>
          )}

          {trend.length > 1 && (
            <Card className="mt-6">
              <CardTitle>Risk trend</CardTitle>
              <CardDescription className="mt-1">Last {trend.length} scans</CardDescription>
              <div className="mt-4 h-48">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={trend} margin={{ left: -20 }}>
                    <CartesianGrid stroke="var(--border-default)" vertical={false} />
                    <XAxis
                      dataKey="scan"
                      stroke="var(--text-tertiary)"
                      fontSize={12}
                      tickLine={false}
                    />
                    <YAxis
                      domain={[0, 100]}
                      stroke="var(--text-tertiary)"
                      fontSize={12}
                      tickLine={false}
                    />
                    <Line
                      type="monotone"
                      dataKey="score"
                      stroke="var(--accent)"
                      strokeWidth={2}
                      dot={{ r: 3, fill: 'var(--accent)' }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card>
          )}
        </>
      )}
    </main>
  )
}
