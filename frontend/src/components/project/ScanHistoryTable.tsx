import { useNavigate } from 'react-router-dom'
import { Card } from '../ui/Card'
import type { OrgScanSummary, ScanSummary } from '../../lib/scansApi'
import { relativeTime } from '../../lib/format'

const STATUS_COLOR: Record<string, string> = {
  queued: 'var(--text-tertiary)',
  running: 'var(--accent)',
  completed: 'var(--success)',
  failed: 'var(--danger)',
  cancelled: 'var(--text-tertiary)',
}

const SEVERITY_ORDER = ['critical', 'high', 'medium', 'low', 'informational']
const SEVERITY_COLOR: Record<string, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  informational: 'var(--sev-info)',
}

function totalFindings(counts: Record<string, number>): number {
  return Object.values(counts).reduce((sum, n) => sum + n, 0)
}

/**
 * The scan-history table — shared between the per-project Scans tab
 * (`showProject={false}`, the project is already implied by the page) and
 * the global Scans page (`showProject={true}`, an OrgScanSummary[] whose
 * rows each carry the project they belong to).
 */
export function ScanHistoryTable({
  scans,
  showProject = false,
}: {
  scans: ScanSummary[] | OrgScanSummary[]
  showProject?: boolean
}) {
  const navigate = useNavigate()

  return (
    <Card className="overflow-x-auto p-0">
      <table className="w-full min-w-[640px] text-left text-body-sm">
        <thead>
          <tr className="border-b border-border-default text-caption font-medium uppercase tracking-wide text-text-tertiary">
            {showProject && <th className="px-6 py-3">Project</th>}
            <th className={showProject ? 'px-4 py-3' : 'px-6 py-3'}>Scan</th>
            <th className="px-4 py-3">Status</th>
            <th className="px-4 py-3">Findings</th>
            <th className="px-6 py-3 text-right">Queued</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border-default">
          {scans.map((s) => (
            <tr
              key={s.id}
              role="button"
              tabIndex={0}
              className="cursor-pointer transition-colors hover:bg-bg-subtle"
              onClick={() => navigate(`/scans/${s.id}`)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') navigate(`/scans/${s.id}`)
              }}
            >
              {showProject && (
                <td className="px-6 py-3 font-medium text-text-primary">
                  {'project_name' in s ? s.project_name : ''}
                </td>
              )}
              <td className={showProject ? 'px-4 py-3' : 'px-6 py-3'}>
                <div className="font-medium text-text-primary">{s.id.slice(0, 8)}</div>
                <div className="text-caption text-text-tertiary">
                  {s.type.replace(/_/g, ' ')}
                  {s.branch ? ` · ${s.branch}` : ''}
                </div>
              </td>
              <td className="px-4 py-3">
                <span
                  className="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-caption font-semibold capitalize"
                  style={{
                    color: STATUS_COLOR[s.status],
                    backgroundColor: `color-mix(in srgb, ${STATUS_COLOR[s.status]} 15%, transparent)`,
                  }}
                >
                  {s.status}
                </span>
              </td>
              <td className="px-4 py-3">
                {totalFindings(s.finding_counts) === 0 ? (
                  <span className="text-text-tertiary">
                    {s.status === 'completed' ? 'None' : '—'}
                  </span>
                ) : (
                  <div className="flex items-center gap-2">
                    {SEVERITY_ORDER.filter((sev) => s.finding_counts[sev] > 0).map((sev) => (
                      <span
                        key={sev}
                        className="inline-flex items-center gap-1 text-caption font-medium"
                        style={{ color: SEVERITY_COLOR[sev] }}
                      >
                        <span
                          className="h-1.5 w-1.5 rounded-full"
                          style={{ backgroundColor: SEVERITY_COLOR[sev] }}
                          aria-hidden="true"
                        />
                        {s.finding_counts[sev]}
                      </span>
                    ))}
                  </div>
                )}
              </td>
              <td className="px-6 py-3 text-right text-text-secondary" title={s.queued_at}>
                {relativeTime(s.queued_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  )
}
