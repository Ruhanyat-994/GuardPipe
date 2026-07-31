import { type FormEvent, useState } from 'react'
import { ExternalLink, GitBranch, KeyRound, Lock } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { Input } from '../ui/Input'
import { GitHubMark } from '../icons/GitHubMark'
import { ApiError } from '../../lib/apiClient'
import { attachRepository, setCredential, type Repository } from '../../lib/projectsApi'

/**
 * FR-PRJ-005: validated before saving — a private repo with no credential
 * comes back as `project.credential_required`, which reveals the PAT field
 * inline instead of a bare error. Shared between `ProjectCreatePage` (a
 * fresh project with no repository yet) and `ProjectSettingsPage` (an
 * existing project attaching/replacing one) — same form either way.
 */
export function RepositoryAttachForm({
  projectId,
  existing,
  onAttached,
}: {
  projectId: string
  existing?: Repository | null
  onAttached?: (repo: Repository) => void
}) {
  const [url, setUrl] = useState('')
  const [needsToken, setNeedsToken] = useState(false)
  const [token, setToken] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [attached, setAttached] = useState<Repository | null>(existing ?? null)

  async function handleAttach(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      if (needsToken && token) {
        // The token is written straight to the credential endpoint and
        // never rendered back — no console.log, no state that lands in the
        // DOM outside this password input.
        await setCredential(projectId, token)
      }
      const repo = await attachRepository(projectId, url)
      setAttached(repo)
      setToken('')
      onAttached?.(repo)
    } catch (err) {
      if (err instanceof ApiError && err.problem.code === 'project.credential_required') {
        setNeedsToken(true)
        setError('This repository is private. Attach a GitHub personal access token to continue.')
      } else {
        setError(
          err instanceof ApiError ? err.problem.detail : 'Something went wrong. Please try again.',
        )
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card>
      <div className="flex items-center gap-2">
        <GitHubMark className="h-4 w-4 text-text-tertiary" />
        <CardTitle className="text-h3">Repository</CardTitle>
      </div>
      <CardDescription className="mt-1">
        {existing ? 'Replace the connected repository.' : 'Optional — attach this any time later.'}
      </CardDescription>

      {attached && !needsToken ? (
        <div className="mt-4 flex items-center justify-between gap-3 rounded-md border border-border-default bg-bg-subtle p-3">
          <div className="flex min-w-0 items-center gap-3">
            <GitHubMark className="h-7 w-7 shrink-0 text-text-primary" />
            <div className="min-w-0">
              <a
                href={attached.url}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-body-sm font-medium text-text-primary hover:text-accent hover:underline"
              >
                <span className="truncate">
                  {attached.owner}/{attached.name}
                </span>
                <ExternalLink className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              </a>
              <div className="mt-0.5 flex items-center gap-3 text-caption text-text-tertiary">
                <span className="flex items-center gap-1">
                  <GitBranch className="h-3 w-3" aria-hidden="true" />
                  {attached.default_branch}
                </span>
                {attached.is_private && (
                  <span className="flex items-center gap-1 text-warning">
                    <Lock className="h-3 w-3" aria-hidden="true" />
                    Private
                  </span>
                )}
              </div>
            </div>
          </div>
          <Button variant="ghost" size="sm" onClick={() => setAttached(null)}>
            Replace
          </Button>
        </div>
      ) : (
        <form onSubmit={handleAttach} className="mt-4 flex flex-col gap-4">
          <div>
            <label htmlFor="repo-url" className="mb-1 block text-body-sm text-text-secondary">
              Repository URL
            </label>
            <div className="relative">
              <GitHubMark className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-text-tertiary" />
              <Input
                id="repo-url"
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://github.com/acme/payments-api"
                className="pl-9"
              />
            </div>
          </div>

          {needsToken && (
            <div>
              <label
                htmlFor="repo-token"
                className="mb-1 flex items-center gap-1.5 text-body-sm text-text-secondary"
              >
                <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
                GitHub personal access token (repo, read-only)
              </label>
              <Input
                id="repo-token"
                type="password"
                autoComplete="off"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="ghp_…"
              />
              <p className="mt-1 text-caption text-text-tertiary">
                Stored encrypted. Never shown again after this — only a masked hint.
              </p>
            </div>
          )}

          {error && (
            <p role="alert" className="text-body-sm text-danger">
              {error}
            </p>
          )}

          <Button
            type="submit"
            variant="secondary"
            loading={submitting}
            disabled={!url}
            className="self-start"
          >
            Attach repository
          </Button>
        </form>
      )}
    </Card>
  )
}
