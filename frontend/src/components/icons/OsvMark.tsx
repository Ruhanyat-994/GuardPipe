import { ShieldHalf } from 'lucide-react'
import { cn } from '../../lib/cn'

/**
 * OSV.dev attribution mark (documentation/09-ui-ux-design-system.md §4.5:
 * "OSV.dev's own mark, on any finding whose evidence includes an OSV
 * advisory ID, and on the depscan node in SupplyChainPipeline"). Built as a
 * labelled badge rather than a traced brand logomark — unlike GitHubMark
 * (Phase 3), this session has no verified source artwork for OSV.dev's
 * actual logo to trace accurately, and shipping a guessed-at SVG path
 * claiming to be a real company's mark would be worse than a plain,
 * honestly-labelled badge that still communicates the same thing: this
 * advisory data came from OSV.dev.
 */
export function OsvMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full bg-bg-subtle px-1.5 py-0.5 text-caption font-semibold text-text-secondary',
        className,
      )}
      title="Advisory data from OSV.dev"
    >
      <ShieldHalf className="h-3 w-3" aria-hidden="true" />
      OSV
    </span>
  )
}
