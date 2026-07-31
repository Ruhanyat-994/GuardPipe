import { type FormEvent, useState } from 'react'
import { FolderGit2, KeyRound } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { Input } from '../ui/Input'
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
  const [attached, setAttached] = useState<{ owner: string; name: string } | null>(
    existing ? { owner: existing.owner, name: existing.name } : null,
  )

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
      setAttached({ owner: repo.owner, name: repo.name })
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
        <FolderGit2 className="h-4 w-4 text-text-tertiary" aria-hidden="true" />
        <CardTitle className="text-h3">Repository</CardTitle>
      </div>
      <CardDescription className="mt-1">
        {existing ? 'Replace the connected repository.' : 'Optional — attach this any time later.'}
      </CardDescription>

      {attached && !needsToken ? (
        <div className="mt-4 flex items-center justify-between">
          <p className="flex items-center gap-1.5 text-body-sm text-success">
            <FolderGit2 className="h-4 w-4" aria-hidden="true" />
            {attached.owner}/{attached.name}
          </p>
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
            <Input
              id="repo-url"
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://github.com/acme/payments-api"
            />
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
