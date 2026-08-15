import { useState } from 'react'
import { ChevronDown, ExternalLink, Sparkles } from 'lucide-react'
import { cn } from '../../lib/cn'
import { buildRepoBlobUrl } from '../../lib/repoLink'
import type { FindingListItem } from '../../lib/scansApi'
import type { Repository } from '../../lib/projectsApi'
import { Tabs } from '../ui/Tabs'

const SEVERITY_COLOR: Record<string, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  informational: 'var(--sev-info)',
}

function locationSummary(f: FindingListItem): string | null {
  const loc = f.location
  if (loc.type === 'file' && loc.path) {
    return loc.line_start ? `${loc.path}:${loc.line_start}` : loc.path
  }
  if (loc.type === 'dependency' && loc.package) {
    return `${loc.ecosystem ?? ''} ${loc.package}${loc.version ? `@${loc.version}` : ''}`.trim()
  }
  if (loc.type === 'image' && loc.path) {
    // containerscan's image-type Location puts the affected package name in
    // `path` (Finding's field doubles up across the file/image shapes —
    // see domain.Location's own doc comment) — this is what makes each
    // finding's specific package legible without opening it, especially
    // important now that several distinct packages routinely share one CVE.
    return loc.layer_digest
      ? `${loc.path} (layer ${String(loc.layer_digest).slice(7, 19)})`
      : loc.path
  }
  return null
}

/**
 * One expandable finding row. Collapsed, it's the flat badge+title+rule-id
 * view this replaces; "More" reveals exactly where the fault is (file:line,
 * plus a link straight to that line in the repository when one can be
 * built), any supporting evidence, and a Description/Remediation tab pair.
 * Remediation is the rule's own deterministic guidance
 * (documentation/03-architecture-overview.md §7.1: "must stand alone
 * without AI") — the AI tab is a placeholder for the AI-authored
 * suggestion Phase 10/11 adds on top of it, not a replacement.
 */
export function FindingRow({
  finding,
  repository,
  gitRef,
}: {
  finding: FindingListItem
  repository: Repository | null
  gitRef: string | null
}) {
  const [expanded, setExpanded] = useState(false)
  const [tab, setTab] = useState<'description' | 'remediation'>('description')

  const summary = locationSummary(finding)
  const blobUrl =
    repository && gitRef && finding.location.type === 'file' && finding.location.path
      ? buildRepoBlobUrl(
          repository.url,
          gitRef,
          finding.location.path,
          finding.location.line_start,
          finding.location.line_end,
        )
      : null

  return (
    <li className="py-3">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center gap-3 text-left"
        aria-expanded={expanded}
      >
        <span
          className="shrink-0 rounded-full px-2 py-0.5 text-caption font-semibold text-white uppercase"
          style={{ backgroundColor: SEVERITY_COLOR[finding.severity] }}
        >
          {finding.severity}
        </span>
        <span className="flex-1 text-body-sm text-text-primary">{finding.title}</span>
        <span className="hidden text-caption text-text-tertiary sm:inline">{finding.rule_id}</span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 text-text-tertiary transition-transform',
            expanded && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </button>

      {expanded && (
        <div className="mt-3 ml-1 flex flex-col gap-3 border-l-2 border-border-default pl-4">
          {summary && (
            <div className="flex flex-wrap items-center gap-2 text-body-sm">
              <code className="rounded bg-bg-subtle px-1.5 py-0.5 font-mono text-caption text-text-secondary">
                {summary}
              </code>
              {blobUrl && (
                <a
                  href={blobUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 text-caption font-medium text-accent hover:underline"
                >
                  View in repository
                  <ExternalLink className="h-3 w-3" aria-hidden="true" />
                </a>
              )}
            </div>
          )}

          {finding.evidence.length > 0 && (
            <div className="flex flex-col gap-1.5">
              {finding.evidence.map((ev, i) => (
                <pre
                  key={i}
                  className="overflow-x-auto rounded-md bg-bg-subtle px-3 py-2 font-mono text-caption text-text-primary"
                >
                  {ev.value}
                </pre>
              ))}
            </div>
          )}

          <div>
            <Tabs
              items={[
                { id: 'description', label: 'Details' },
                { id: 'remediation', label: 'Remediation' },
              ]}
              active={tab}
              onChange={(id) => setTab(id as 'description' | 'remediation')}
            />
            <div className="pt-3 text-body-sm text-text-secondary">
              {tab === 'description' && <p>{finding.description}</p>}
              {tab === 'remediation' && (
                <div className="flex flex-col gap-2">
                  <p>{finding.remediation}</p>
                  <p className="flex items-center gap-1.5 text-caption text-text-tertiary">
                    <Sparkles className="h-3 w-3" aria-hidden="true" />
                    Deterministic guidance from the rule today — AI-authored, context-aware
                    suggestions land in a later phase.
                  </p>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </li>
  )
}
