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
