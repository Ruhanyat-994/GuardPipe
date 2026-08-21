import { AlertTriangle, Info } from 'lucide-react'
import { humanizeErrorReason } from '../../lib/engineRunMessages'
import type { Job } from '../../lib/scansApi'

/**
 * Names the failed/skipped engines plainly rather than letting a partial
 * scan look complete (documentation/09-ui-ux-design-system.md §4.2:
 * "the state people forget"). First real use — Phase 6 is the first phase
 * where a partial result is even possible (one job failing never fails
 * the whole scan, FR-ORC-006/NFR-REL-001).
 *
 * Failed and skipped are deliberately rendered as two different banners,
 * not one merged list. A skip is Applicable() correctly finding nothing
 * for that engine to look at (e.g. containerscan on a project with no
 * Dockerfile) — a normal, honest outcome, not a degraded one. Lumping it
 * into the same orange "partial result" warning as a real failure was
 * itself the "not eye soothing" bug: both used the identical
 * AlertTriangle-on-warning treatment, so a perfectly healthy skip read as
 * alarming as an engine that actually broke (principle 7, "calm, not
 * alarming").
 */
export function PartialResultBanner({ jobs }: { jobs: Job[] }) {
  const failed = jobs.filter((j) => j.status === 'failed')
  const skipped = jobs.filter((j) => j.status === 'skipped')

  if (failed.length === 0 && skipped.length === 0) {
    return null
  }

  return (
    <div className="mb-4 flex flex-col gap-3">
      {failed.length > 0 && (
        <div className="flex items-start gap-3 rounded-lg border border-warning/30 bg-warning/5 p-4">
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
          <div className="text-body-sm text-text-primary">
            <p className="font-medium">This scan completed with a partial result.</p>
            <ul className="mt-1 list-inside list-disc text-text-secondary">
              {failed.map((j) => (
                <li key={j.id}>
                  <span className="font-medium capitalize">{j.engine}</span> failed (
                  {humanizeErrorReason(j.error_reason)}) — its findings are not included.
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}

      {skipped.length > 0 && (
        <div className="flex items-start gap-3 rounded-lg border border-border-default bg-bg-subtle p-4">
          <Info className="mt-0.5 h-5 w-5 shrink-0 text-text-tertiary" aria-hidden="true" />
          <div className="text-body-sm text-text-primary">
            <p className="font-medium">
              {skipped.length === 1
                ? '1 check had nothing to look at'
                : `${skipped.length} checks had nothing to look at`}
            </p>
            <ul className="mt-1 list-inside list-disc text-text-secondary">
              {skipped.map((j) => (
                <li key={j.id}>
                  <span className="font-medium capitalize">{j.engine}</span>
                  {j.skip_reason ? ` — ${j.skip_reason}` : ' was skipped for this scan'}.
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </div>
  )
}
