import { KeyRound } from 'lucide-react'
import { Link } from 'react-router-dom'
import type { Project } from '../../lib/projectsApi'

/**
 * "This project's GitHub token has expired, reconnect it" — shown on every
 * `/projects/:id/*` tab (mounted once in ProjectLayout, above the Outlet)
 * rather than only on a scan's own findings, so it's visible the moment
 * someone opens the project, not just after they happen to look at a failed
 * scan's PartialResultBanner. Deliberately does not say the project or its
 * history was touched — nothing here ever deletes or archives anything, the
 * repository row and every past scan/finding are exactly where they were.
 */
export function CredentialInvalidBanner({ project }: { project: Project }) {
  const repo = project.repository
  if (!repo?.credential_invalid) {
    return null
  }

  return (
    <div className="mx-auto mb-4 flex max-w-6xl items-start gap-3 rounded-lg border border-danger/30 bg-danger/5 px-6 py-4">
      <KeyRound className="mt-0.5 h-5 w-5 shrink-0 text-danger" aria-hidden="true" />
      <div className="text-body-sm text-text-primary">
        <p className="font-medium">This project's GitHub access token is no longer valid.</p>
        <p className="mt-0.5 text-text-secondary">
          Scans against{' '}
          <span className="font-medium">
            {repo.owner}/{repo.name}
          </span>{' '}
          will keep failing until you reattach one — every past scan and finding is untouched, and
          once a new token is connected the project picks back up exactly where it left off.{' '}
          <Link
            to={`/projects/${project.id}/settings`}
            className="font-medium text-danger underline hover:no-underline"
          >
            Reconnect a token
          </Link>
        </p>
      </div>
    </div>
  )
}
