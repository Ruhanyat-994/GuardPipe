import { useEffect, useMemo, useState } from 'react'
import { BookOpen, ChevronDown, ChevronRight } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Button } from '../components/ui/Button'
import { useAuthStore } from '../stores/authStore'
import { ApiError } from '../lib/apiClient'
import { ENGINE_META } from '../lib/engines'
import {
  listRules,
  setRuleEnabled,
  type Engine,
  type Rule,
  type Severity,
  type Tier,
} from '../lib/rulesApi'
import { cn } from '../lib/cn'

/**
 * Screen 11 — Rules catalogue (documentation/09-ui-ux-design-system.md
 * §5.8: "rules as a filterable table with an expandable detail row").
 * Replaces the Phase 3 sidebar placeholder — the first /rules load to show
 * real ingested data (BUILD_GUIDE.md Phase 5). It will legitimately render
 * empty against a fresh backend today: the `rules` table is seeded from
 * each engine's own code registry at startup, and no engine exists yet
 * (codescan/depscan land Phase 6+) — same honest-empty-state posture as
 * ProjectTargetsPage before Phase 3 shipped real targets.
 */

const SEVERITY_LABEL: Record<Severity, string> = {
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
  informational: 'Informational',
}

// var(--sev-*) custom properties use "info", not "informational"
// (components/dashboard/SeverityStatTile.tsx) — this maps the backend's
// full word onto that shorter CSS variable suffix.
const SEVERITY_VAR: Record<Severity, string> = {
  critical: 'critical',
  high: 'high',
  medium: 'medium',
  low: 'low',
  informational: 'info',
}

function SeverityBadge({ severity }: { severity: Severity }) {
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-caption font-semibold text-white"
      style={{ backgroundColor: `var(--sev-${SEVERITY_VAR[severity]})` }}
    >
      {SEVERITY_LABEL[severity]}
    </span>
  )
}

function TierBadge({ tier }: { tier: Tier }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-caption font-semibold capitalize',
        tier === 'core' ? 'bg-accent/10 text-accent' : 'bg-bg-subtle text-text-secondary',
      )}
    >
      {tier}
    </span>
  )
}

export function RulesPage() {
  const user = useAuthStore((s) => s.user)
  const isAdmin = user?.role === 'admin'

  const [rules, setRules] = useState<Rule[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [pendingId, setPendingId] = useState<string | null>(null)

  const [engineFilter, setEngineFilter] = useState<Engine | ''>('')
  const [tierFilter, setTierFilter] = useState<Tier | ''>('')
  const [severityFilter, setSeverityFilter] = useState<Severity | ''>('')

  function refresh() {
    listRules({
      engine: engineFilter || undefined,
      tier: tierFilter || undefined,
      severity: severityFilter || undefined,
    })
      .then((res) => setRules(res.data))
      .catch((err: unknown) => {
        setError(
          err instanceof ApiError ? err.problem.detail : 'Could not load the rules catalogue.',
        )
      })
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [engineFilter, tierFilter, severityFilter])

  const hasFilters = engineFilter !== '' || tierFilter !== '' || severityFilter !== ''

  const rows = useMemo(() => rules ?? [], [rules])

  async function toggleEnabled(rule: Rule) {
    setPendingId(rule.id)
    try {
      const updated = await setRuleEnabled(rule.id, !rule.enabled)
      setRules((prev) => prev?.map((r) => (r.id === updated.id ? updated : r)) ?? prev)
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not update the rule.')
    } finally {
      setPendingId(null)
    }
  }

  return (
    <main className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">Rules catalogue</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        What GuardPipe actually checks — every rule any engine can raise, its default severity, and
        whether it's currently enabled.
      </p>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      <Card className="mb-4">
        <div className="flex flex-wrap items-center gap-3">
          <FilterSelect
            label="Engine"
            value={engineFilter}
            onChange={(v) => setEngineFilter(v as Engine | '')}
            options={Object.entries(ENGINE_META).map(([id, meta]) => ({
              value: id,
              label: meta.label,
            }))}
          />
          <FilterSelect
            label="Tier"
            value={tierFilter}
            onChange={(v) => setTierFilter(v as Tier | '')}
            options={[
              { value: 'core', label: 'Core' },
              { value: 'stretch', label: 'Stretch' },
            ]}
          />
          <FilterSelect
            label="Severity"
            value={severityFilter}
            onChange={(v) => setSeverityFilter(v as Severity | '')}
            options={Object.entries(SEVERITY_LABEL).map(([value, label]) => ({ value, label }))}
          />
        </div>
      </Card>

      <Card>
        <CardTitle className="text-h3">Rules {rules !== null && `(${rows.length})`}</CardTitle>

        {rules === null && !error && <CardDescription className="mt-2">Loading…</CardDescription>}

        {rules !== null && rows.length === 0 && (
          <div className="mt-4 flex flex-col items-center gap-2 py-10 text-center">
            <BookOpen className="h-8 w-8 text-text-tertiary" aria-hidden="true" />
            <CardDescription>
              {hasFilters
                ? 'No rules match these filters.'
                : 'No rules in the catalogue yet — it fills in as each scanning engine ships and registers its rules (Phase 6 onward).'}
            </CardDescription>
          </div>
        )}

        {rules !== null && rows.length > 0 && (
          <ul className="mt-4 flex flex-col divide-y divide-border-default">
            {rows.map((rule) => {
              const expanded = expandedId === rule.id
              const Icon = ENGINE_META[rule.engine].icon
              return (
                <li key={rule.id}>
                  <button
                    type="button"
                    onClick={() => setExpandedId(expanded ? null : rule.id)}
                    className="flex w-full items-center gap-3 py-3 text-left"
                    aria-expanded={expanded}
                  >
                    {expanded ? (
                      <ChevronDown
                        className="h-4 w-4 shrink-0 text-text-tertiary"
                        aria-hidden="true"
                      />
                    ) : (
                      <ChevronRight
                        className="h-4 w-4 shrink-0 text-text-tertiary"
                        aria-hidden="true"
                      />
                    )}
                    <Icon className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden="true" />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-body-sm text-text-primary">{rule.title}</div>
                      <div className="truncate font-mono text-caption text-text-tertiary">
                        {rule.id}
                      </div>
                    </div>
                    <TierBadge tier={rule.tier} />
                    <SeverityBadge severity={rule.default_severity} />
                    <span
                      className={cn(
                        'shrink-0 rounded-full px-2 py-0.5 text-caption font-semibold',
                        rule.enabled
                          ? 'bg-success/10 text-success'
                          : 'bg-bg-subtle text-text-tertiary',
                      )}
                    >
                      {rule.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </button>

                  {expanded && (
                    <div className="mb-3 ml-7 flex flex-col gap-3 rounded-md bg-bg-subtle p-4">
                      <p className="text-body-sm text-text-primary">{rule.description}</p>
                      <div>
                        <div className="text-caption font-semibold text-text-tertiary uppercase">
                          Remediation
                        </div>
                        <p className="mt-1 text-body-sm text-text-primary">{rule.remediation}</p>
                      </div>
                      {(rule.cwe.length > 0 || rule.owasp.length > 0) && (
                        <div className="flex flex-wrap gap-4 text-caption text-text-tertiary">
                          {rule.cwe.length > 0 && <span>CWE: {rule.cwe.join(', ')}</span>}
                          {rule.owasp.length > 0 && <span>OWASP: {rule.owasp.join(', ')}</span>}
                        </div>
                      )}
                      {isAdmin && (
                        <div>
                          <Button
                            variant={rule.enabled ? 'destructive' : 'secondary'}
                            size="sm"
                            loading={pendingId === rule.id}
                            onClick={() => void toggleEnabled(rule)}
                          >
                            {rule.enabled ? 'Disable rule' : 'Enable rule'}
                          </Button>
                        </div>
                      )}
                    </div>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </Card>
    </main>
  )
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <label className="flex items-center gap-2 text-body-sm text-text-secondary">
      {label}
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cn(
          'h-9 rounded-md border border-border-default bg-bg-surface px-2 text-body-sm text-text-primary',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2',
        )}
      >
        <option value="">All</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}
