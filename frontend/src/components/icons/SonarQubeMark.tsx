import { ScanSearch } from 'lucide-react'
import { cn } from '../../lib/cn'

/**
 * SonarQube attribution mark (documentation/09-ui-ux-design-system.md §4.5:
 * "SonarQube's own logomark, on any finding whose RuleID is namespaced
 * codescan.sonarqube.*, and on the codescan node in SupplyChainPipeline" —
 * added Phase 7, ADR-0011, reversing the earlier "codescan gets no external
 * brand mark" note now that it wraps a self-hosted SonarQube instance
 * instead of running its own SAST). Built as a labelled badge rather than a
 * traced brand logomark, same reasoning as OsvMark: this session has no
 * verified source artwork for SonarQube's actual logo to trace accurately,
 * and a guessed-at SVG path claiming to be a real company's mark would be
 * worse than a plain, honestly-labelled badge that still communicates the
 * same thing — this analysis came from SonarQube.
 */
export function SonarQubeMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full bg-bg-subtle px-1.5 py-0.5 text-caption font-semibold text-text-secondary',
        className,
      )}
      title="Analysis from SonarQube"
    >
      <ScanSearch className="h-3 w-3" aria-hidden="true" />
      SonarQube
    </span>
  )
}
