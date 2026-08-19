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
 * A Kubernetes finding isn't "at a line" the way SAST is — it's about a
 * resource, in a namespace, in a container, at a config field. Rather than
 * force it through the same file:line model, this builds the breadcrumb a
 * "Resource Inspector" needs: namespace -> Kind/name -> container ->
 * field path, each segment only present when the Location actually carries
 * it. `value` (the literal offending setting, e.g. "/var/run/docker.sock")
 * is deliberately kept separate — callers render it as evidence, not as
 * one more breadcrumb segment, since it's what was found, not where.
 */
function k8sResourcePath(loc: FindingListItem['location']): string[] {
  if (loc.type !== 'k8s' || !loc.kind || !loc.name) return []
  const segments: string[] = []
  if (loc.namespace) segments.push(`namespace/${loc.namespace}`)
  segments.push(`${loc.kind}/${loc.name}`)
  if (loc.container) segments.push(`container/${loc.container}`)
  if (loc.field_path) segments.push(loc.field_path)
  return segments
}

/**
 * A finding only gets a "View in repository" link when its Location names a
 * real, navigable line in the checkout: `file` (codescan, depscan,
 * containerscan's Dockerfile-misconfig findings — both share the same
 * file-type shape) or `k8s` (k8sscan; File is real for both a raw manifest
 * and a Helm-rendered one — see domain.Location's own doc comment).
 * Deliberately no link for `dependency` (a CVE lives in the resolved
 * package's own code, not at a line of this repository) or `image`
 * (containerscan's Trivy vulnerability/secret findings point at an image
 * layer, not a source file) — those are architectural findings, shown via
 * locationSummary above instead of a link that would point somewhere
 * wrong or nowhere.
 */
function buildFindingBlobUrl(
  loc: FindingListItem['location'],
  repository: Repository | null,
  gitRef: string | null,
): string | null {
  if (!repository || !gitRef) return null
  if (loc.type === 'file' && loc.path) {
    return buildRepoBlobUrl(repository.url, gitRef, loc.path, loc.line_start, loc.line_end)
  }
  if (loc.type === 'k8s' && loc.file) {
    // line_start is only ever set for a raw manifest (a document's real
    // start line in `file`) — 0/absent for a Helm-sourced finding, so this
    // degrades to a plain link to the template file with no line anchor
    // rather than an anchor pointing at the wrong (rendered-output) line.
    return buildRepoBlobUrl(repository.url, gitRef, loc.file, loc.line_start, loc.line_end)
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
  const resourcePath = k8sResourcePath(finding.location)
  const blobUrl = buildFindingBlobUrl(finding.location, repository, gitRef)
  const impact = finding.metadata?.impact
  const attackPath = finding.metadata?.attack_path

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
          {resourcePath.length > 0 ? (
            <div className="flex flex-col gap-1.5">
              <div className="flex flex-wrap items-center gap-1.5 text-body-sm">
                {resourcePath.map((segment, i) => (
                  <span key={i} className="flex items-center gap-1.5">
                    {i > 0 && (
                      <span className="text-text-tertiary" aria-hidden="true">
                        →
                      </span>
                    )}
                    <code className="rounded bg-bg-subtle px-1.5 py-0.5 font-mono text-caption text-text-secondary">
                      {segment}
                    </code>
                  </span>
                ))}
                {blobUrl && (
                  <a
                    href={blobUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 text-caption font-medium text-accent hover:underline"
                  >
                    View manifest
                    <ExternalLink className="h-3 w-3" aria-hidden="true" />
                  </a>
                )}
              </div>
              {finding.location.value && (
                <pre className="overflow-x-auto rounded-md bg-bg-subtle px-3 py-2 font-mono text-caption text-text-primary">
                  {finding.location.value}
                </pre>
              )}
            </div>
          ) : (
            summary && (
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
            )
          )}

          {(impact || (attackPath && attackPath.length > 0)) && (
            <div className="flex flex-col gap-2 rounded-md bg-bg-subtle px-3 py-2.5">
              {impact && (
                <div>
                  <p className="text-caption font-semibold text-text-secondary">Why this matters</p>
                  <p className="mt-0.5 text-body-sm text-text-primary">{impact}</p>
                </div>
              )}
              {attackPath && attackPath.length > 0 && (
                <div>
                  <p className="text-caption font-semibold text-text-secondary">Attack path</p>
                  <div className="mt-1 flex flex-wrap items-center gap-1.5 text-body-sm text-text-primary">
                    {attackPath.map((stage, i) => (
                      <span key={i} className="flex items-center gap-1.5">
                        {i > 0 && (
                          <span className="text-text-tertiary" aria-hidden="true">
                            →
                          </span>
                        )}
                        <span className="rounded-full bg-bg-surface px-2 py-0.5 text-caption">
                          {stage}
                        </span>
                      </span>
                    ))}
                  </div>
                </div>
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
