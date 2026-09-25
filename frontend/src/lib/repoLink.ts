import type { FindingListItem } from './scansApi'
import type { Repository } from './projectsApi'

/**
 * Builds a clickable link straight to the offending line in the project's
 * repository — e.g. `https://github.com/{owner}/{repo}/blob/{ref}/{path}#L42`.
 * Only GitHub is supported today (the only adapter this codebase has,
 * documentation/03-architecture-overview.md's module list) — an
 * unrecognised host returns null rather than guessing a URL shape.
 */
export function buildRepoBlobUrl(
  repositoryUrl: string,
  ref: string,
  path: string,
  lineStart?: number,
  lineEnd?: number,
): string | null {
  const cleaned = repositoryUrl.replace(/\.git$/, '').replace(/\/+$/, '')
  if (!/^https?:\/\/(www\.)?github\.com\/[^/]+\/[^/]+$/.test(cleaned)) return null

  const encodedPath = path.split('/').filter(Boolean).map(encodeURIComponent).join('/')

  let anchor = ''
  if (lineStart) {
    anchor = lineEnd && lineEnd !== lineStart ? `#L${lineStart}-L${lineEnd}` : `#L${lineStart}`
  }

  return `${cleaned}/blob/${encodeURIComponent(ref)}/${encodedPath}${anchor}`
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
export function buildFindingBlobUrl(
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
