import { ExternalLink } from 'lucide-react'
import { cn } from '../../lib/cn'

/**
 * `CweChip` / `CveChip` (documentation/09-ui-ux-design-system.md §4.2):
 * links to MITRE / NVD. Both render nothing for an empty list — most
 * findings carry zero or one of these, not both.
 */
export function CweChip({ ids, className }: { ids: string[]; className?: string }) {
  return (
    <>
      {ids.map((cwe) => (
        <a
          key={cwe}
          href={`https://cwe.mitre.org/data/definitions/${encodeURIComponent(cwe.replace(/^CWE-/i, ''))}.html`}
          target="_blank"
          rel="noreferrer"
          className={cn(
            'inline-flex items-center gap-1 rounded-full border border-border-default bg-bg-subtle px-2 py-0.5',
            'text-caption font-medium text-text-secondary hover:text-accent',
            className,
          )}
        >
          {cwe}
          <ExternalLink className="h-3 w-3" aria-hidden="true" />
        </a>
      ))}
    </>
  )
}

export function CveChip({ ids, className }: { ids: string[]; className?: string }) {
  return (
    <>
      {ids.map((cve) => (
        <a
          key={cve}
          href={`https://nvd.nist.gov/vuln/detail/${encodeURIComponent(cve)}`}
          target="_blank"
          rel="noreferrer"
          className={cn(
            'inline-flex items-center gap-1 rounded-full border border-border-default bg-bg-subtle px-2 py-0.5',
            'text-caption font-medium text-text-secondary hover:text-accent',
            className,
          )}
        >
          {cve}
          <ExternalLink className="h-3 w-3" aria-hidden="true" />
        </a>
      ))}
    </>
  )
}
