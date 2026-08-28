import { Sparkles } from 'lucide-react'
import { cn } from '../../lib/cn'

/**
 * `PatchDiff` (documentation/09-ui-ux-design-system.md §4.2): unified ·
 * split; verified · unverified badge. Always carries the AI banner — a
 * generated patch is never shown without attributing it. Only "unified"
 * is built (a "split" side-by-side view is a Stretch-level nicety this
 * pass doesn't add). `patchStatus` is "unverified" or "not_applicable" —
 * never "verified" today; see ai.Suggestion's own backend doc comment for
 * why (verifying needs a workspace checkout that's already gone by the
 * time enrichment runs).
 */
export function PatchDiff({
  diff,
  patchStatus,
  className,
}: {
  diff: string
  patchStatus?: 'unverified' | 'not_applicable' | null
  className?: string
}) {
  if (!diff) return null
  const lines = diff.split('\n')

  return (
    <div className={cn('overflow-hidden rounded-md border border-border-default', className)}>
      <div className="flex items-center justify-between gap-2 border-b border-border-default bg-bg-subtle px-3 py-1.5">
        <span className="inline-flex items-center gap-1.5 text-caption font-semibold text-ai">
          <Sparkles className="h-3 w-3" aria-hidden="true" />
          AI-suggested patch
        </span>
        {patchStatus === 'unverified' && (
          <span className="rounded-full bg-warning/10 px-2 py-0.5 text-caption font-medium text-warning">
            Unverified — review before applying
          </span>
        )}
      </div>
      <pre className="overflow-x-auto bg-bg-base p-3 font-mono text-code leading-relaxed">
        {lines.map((line, i) => (
          <div
            key={i}
            className={cn(
              'px-1',
              line.startsWith('+') && !line.startsWith('+++') && 'bg-success/10 text-success',
              line.startsWith('-') && !line.startsWith('---') && 'bg-danger/10 text-danger',
              line.startsWith('@@') && 'font-semibold text-accent',
            )}
          >
            {line.length > 0 ? line : ' '}
          </div>
        ))}
      </pre>
    </div>
  )
}
