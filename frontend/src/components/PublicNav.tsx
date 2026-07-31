import { Link } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'

/**
 * Floating pill nav, direction adapted from tridentsecurity.io
 * (documentation/09-ui-ux-design-system.md §5.9) — shared by Landing,
 * Guides, and Blog. Three-column layout: logo left, primary links centred
 * in the pill, the call-to-action pinned right.
 */
export function PublicNav() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)

  return (
    <header className="sticky top-4 z-10 mx-auto flex w-full max-w-3xl items-center justify-between rounded-full border border-border-default bg-bg-surface/90 px-6 py-3 shadow-md backdrop-blur relative">
      <Link to="/" className="text-h3 font-semibold text-text-primary">
        GuardPipe
      </Link>

      <nav className="absolute left-1/2 hidden -translate-x-1/2 items-center gap-6 text-body-sm font-semibold sm:flex">
        <Link to="/guides" className="text-text-secondary hover:text-text-primary">
          Guides
        </Link>
        <Link to="/blog" className="text-text-secondary hover:text-text-primary">
          Blog
        </Link>
        {!isAuthenticated && (
          <Link to="/login" className="text-text-secondary hover:text-text-primary">
            Sign in
          </Link>
        )}
      </nav>

      {isAuthenticated ? (
        <Link
          to="/projects"
          className="rounded-md bg-accent px-3 py-1.5 text-body-sm font-medium text-text-inverse hover:opacity-90"
        >
          Dashboard
        </Link>
      ) : (
        <Link
          to="/register"
          className="rounded-md bg-accent px-3 py-1.5 text-body-sm font-medium text-text-inverse hover:opacity-90"
        >
          Get started
        </Link>
      )}
    </header>
  )
}
