import { Sparkles } from 'lucide-react'
import { cn } from '../../lib/cn'

/**
 * Gemini attribution mark (documentation/09-ui-ux-design-system.md §4.5,
 * extended Phase 11: "Gemini's own mark... on the docreview node in
 * SupplyChainPipeline" — docreview is the one engine where AI review is the
 * entire output, not an enrichment). Built as a labelled badge rather than a
 * traced brand logomark, same reasoning as OsvMark/SonarQubeMark: this
 * session has no verified source artwork for Gemini's actual multi-colour
 * sparkle logo to trace accurately, and a guessed-at SVG path claiming to be
 * a real company's mark would be worse than a plain, honestly-labelled badge
 * that still communicates the same thing — this review came from Gemini.
 * Reuses the `Sparkles` icon FindingRow's own "AI-generated" chip already
 * uses, keeping one consistent "this is AI" visual language across the app
 * rather than a second, competing one.
 */
export function GeminiMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full bg-bg-subtle px-1.5 py-0.5 text-caption font-semibold text-text-secondary',
        className,
      )}
      title="Review from Gemini"
    >
      <Sparkles className="h-3 w-3" aria-hidden="true" />
      Gemini
    </span>
  )
}
