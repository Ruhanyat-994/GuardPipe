import { AlertTriangle } from 'lucide-react'
import type { Job } from '../../lib/scansApi'

/**
 * Names the failed/skipped engines plainly rather than letting a partial
 * scan look complete (documentation/09-ui-ux-design-system.md §4.2:
 * "the state people forget"). First real use — Phase 6 is the first phase
 * where a partial result is even possible (one job failing never fails
 * the whole scan, FR-ORC-006/NFR-REL-001).
 */
export function PartialResultBanner({ jobs }: { jobs: Job[] }) {
  const failed = jobs.filter((j) => j.status === 'failed')
  const skipped = jobs.filter((j) => j.status === 'skipped')

  if (failed.length === 0 && skipped.length === 0) {
    return null
  }

  return (
    <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/30 bg-warning/5 p-4">
      <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
      <div className="text-body-sm text-text-primary">
        <p className="font-medium">This scan completed with a partial result.</p>
        <ul className="mt-1 list-inside list-disc text-text-secondary">
          {failed.map((j) => (
            <li key={j.id}>
              <span className="font-medium capitalize">{j.engine}</span> failed
              {j.error_reason ? ` (${j.error_reason.replace(/_/g, ' ')})` : ''} — its findings are
              not included.
            </li>
          ))}
          {skipped.map((j) => (
            <li key={j.id}>
              <span className="font-medium capitalize">{j.engine}</span> was skipped
              {j.skip_reason ? `: ${j.skip_reason}` : ''}.
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
