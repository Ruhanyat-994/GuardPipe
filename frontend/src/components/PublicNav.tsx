import { Link } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'

/**
 * Floating pill nav, direction adapted from tridentsecurity.io
 * (documentation/09-ui-ux-design-system.md §5.9) — shared by Landing,
 * Guides, and Blog.
 */
export function PublicNav() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)

  return (
    <header className="sticky top-4 z-10 mx-auto flex w-full max-w-3xl items-center justify-between rounded-full border border-border-default bg-bg-surface/90 px-6 py-3 shadow-md backdrop-blur">
      <Link to="/" className="text-h3 font-semibold text-text-primary">
        GuardPipe
      </Link>
      <nav className="flex items-center gap-5 text-body-sm">
        <Link to="/guides" className="text-text-secondary hover:text-text-primary">
          Guides
        </Link>
        <Link to="/blog" className="text-text-secondary hover:text-text-primary">
          Blog
        </Link>
        {isAuthenticated ? (
          <Link
            to="/projects"
            className="rounded-md bg-accent px-3 py-1.5 font-medium text-text-inverse hover:opacity-90"
          >
            Dashboard
          </Link>
        ) : (
          <>
            <Link to="/login" className="text-text-secondary hover:text-text-primary">
              Sign in
            </Link>
            <Link
              to="/register"
              className="rounded-md bg-accent px-3 py-1.5 font-medium text-text-inverse hover:opacity-90"
            >
              Get started
            </Link>
          </>
        )}
      </nav>
    </header>
  )
}
