import { useNavigate } from 'react-router-dom'
import {
  FileText,
  Code2,
  Package,
  Container as ContainerIcon,
  Boxes,
  Workflow,
  ShieldAlert,
} from 'lucide-react'
import {
  CartesianGrid,
  Line,
  LineChart,
  Pie,
  PieChart,
  Cell,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from 'recharts'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { RiskGauge } from '../components/dashboard/RiskGauge'
import { SeverityStatTile, type Severity } from '../components/dashboard/SeverityStatTile'
import { PipelineStage, type StageStatus } from '../components/dashboard/PipelineStage'
import { useAuthStore } from '../stores/authStore'

/**
 * Hardcoded preview of the real Phase 8 dashboard
 * (documentation/09-ui-ux-design-system.md §5.1, Screen 4 — the hero
 * screen). Layout and colour treatment modelled directly on the
 * Tenable/SecurityCenter reference screenshots: solid severity stat tiles,
 * donut breakdowns, a trend line, a top-findings list. Every number here is
 * fixed sample data — there is no project or scan yet (that's Phase 3/6) —
 * so the page says so explicitly rather than letting fake findings pass as
 * real ones.
 */

const SEVERITY_COUNTS: { severity: Severity; count: number; delta: number }[] = [
  { severity: 'critical', count: 2, delta: -1 },
  { severity: 'high', count: 9, delta: -3 },
  { severity: 'medium', count: 21, delta: 4 },
  { severity: 'low', count: 14, delta: -2 },
  { severity: 'info', count: 6, delta: 0 },
]

const DONUT_COLORS: Record<Severity, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  info: 'var(--sev-info)',
}

const PIPELINE: { label: string; detail: string; status: StageStatus; icon: typeof FileText }[] = [
  { label: 'Docs', detail: 'failed', status: 'failed', icon: FileText },
  { label: 'Code', detail: '18 high', status: 'high', icon: Code2 },
  { label: 'Deps', detail: '11 critical', status: 'critical', icon: Package },
  { label: 'Containers', detail: 'skipped', status: 'skipped', icon: ContainerIcon },
  { label: 'K8s', detail: '14 critical', status: 'critical', icon: Boxes },
  { label: 'CI/CD', detail: '6 high', status: 'high', icon: Workflow },
  { label: 'Pentest', detail: 'not run', status: 'not_run', icon: ShieldAlert },
]

const TOP_FINDINGS: { severity: Severity; title: string; engine: string }[] = [
  { severity: 'critical', title: 'Hardcoded AWS access key', engine: 'code' },
  { severity: 'critical', title: 'ClusterRoleBinding grants cluster-admin', engine: 'k8s' },
  { severity: 'high', title: 'SQL query built by string concatenation', engine: 'code' },
  { severity: 'high', title: 'lodash 4.17.15 — CVE-2021-23337', engine: 'deps' },
  { severity: 'medium', title: 'Missing Content-Security-Policy header', engine: 'pentest' },
]

const TREND = [
  { scan: '1', score: 81 },
  { scan: '2', score: 78 },
  { scan: '3', score: 83 },
  { scan: '4', score: 76 },
  { scan: '5', score: 74 },
  { scan: '6', score: 72 },
  { scan: '7', score: 74 },
  { scan: '8', score: 68 },
]

export function DashboardPage() {
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="min-h-screen bg-bg-base">
      <header
        className="px-6 py-4 text-text-inverse"
        style={{ background: 'linear-gradient(135deg, var(--glow-2), var(--glow-1))' }}
      >
        <div className="mx-auto flex max-w-6xl items-center justify-between">
          <span className="text-h3 font-semibold">GuardPipe</span>
          <div className="flex items-center gap-4 text-body-sm">
            <span className="text-white/80">
              {user?.email} · {user?.role}
            </span>
            <Button variant="secondary" size="sm" onClick={handleLogout}>
              Sign out
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-accent/30 bg-accent/10 px-3 py-1 text-caption font-semibold text-accent">
          Preview — sample data. Real scanning lands in later phases.
        </div>

        <div className="mb-6 flex items-center justify-between">
          <div>
            <h1 className="text-h1 text-text-primary">Payments API</h1>
            <p className="text-body-sm text-text-secondary">
              github.com/acme/payments-api · main · a3f9c21 · last scan 4 min ago
            </p>
          </div>
          <Button disabled>Run Scan</Button>
        </div>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[auto_1fr]">
          <Card className="flex items-center justify-center">
            <RiskGauge score={68} verdict="block" />
          </Card>

          <Card>
            <CardTitle>Current findings</CardTitle>
            <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-5">
              {SEVERITY_COUNTS.map((s) => (
                <SeverityStatTile
                  key={s.severity}
                  severity={s.severity}
                  count={s.count}
                  delta={s.delta}
                />
              ))}
            </div>

            <div className="mt-6 flex flex-col items-center gap-4 sm:flex-row">
              <div className="h-40 w-40 shrink-0">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={SEVERITY_COUNTS}
                      dataKey="count"
                      nameKey="severity"
                      innerRadius={40}
                      outerRadius={70}
                      paddingAngle={2}
                    >
                      {SEVERITY_COUNTS.map((s) => (
                        <Cell key={s.severity} fill={DONUT_COLORS[s.severity]} stroke="none" />
                      ))}
                    </Pie>
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <ul className="flex flex-col gap-1.5 text-body-sm">
                {SEVERITY_COUNTS.map((s) => (
                  <li key={s.severity} className="flex items-center gap-2">
                    <span
                      className="h-2.5 w-2.5 rounded-full"
                      style={{ backgroundColor: DONUT_COLORS[s.severity] }}
                      aria-hidden="true"
                    />
                    <span className="capitalize text-text-secondary">{s.severity}</span>
                    <span className="font-medium text-text-primary">{s.count}</span>
                  </li>
                ))}
              </ul>
            </div>
          </Card>
        </div>

        <Card className="mt-6">
          <CardTitle>Supply chain</CardTitle>
          <div className="mt-4 flex flex-wrap gap-3">
            {PIPELINE.map((stage) => (
              <PipelineStage key={stage.label} {...stage} />
            ))}
          </div>
        </Card>

        <div className="mt-6 flex items-center justify-between rounded-md border border-warning/30 bg-warning/10 px-4 py-3 text-body-sm text-text-primary">
          <span>⚠ Document review failed: AI service unavailable.</span>
          <Button variant="secondary" size="sm" disabled>
            Retry
          </Button>
        </div>

        <div className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-2">
          <Card>
            <div className="flex items-center justify-between">
              <CardTitle>Top findings</CardTitle>
              <Button variant="ghost" size="sm" disabled>
                View all →
              </Button>
            </div>
            <ul className="mt-4 flex flex-col divide-y divide-border-default">
              {TOP_FINDINGS.map((f) => (
                <li key={f.title} className="flex items-center gap-3 py-3">
                  <span
                    className="rounded-full px-2 py-0.5 text-caption font-semibold text-white uppercase"
                    style={{ backgroundColor: `var(--sev-${f.severity})` }}
                  >
                    {f.severity}
                  </span>
                  <span className="flex-1 text-body-sm text-text-primary">{f.title}</span>
                  <span className="text-caption text-text-tertiary">{f.engine}</span>
                </li>
              ))}
            </ul>
          </Card>

          <Card>
            <CardTitle>Risk trend</CardTitle>
            <CardDescription className="mt-1">Last {TREND.length} scans</CardDescription>
            <div className="mt-4 h-48">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={TREND} margin={{ left: -20 }}>
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
        </div>
      </main>
    </div>
  )
}
