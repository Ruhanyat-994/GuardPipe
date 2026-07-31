import { Link } from 'react-router-dom'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { guides } from '../content/guides'

/** documentation/09-ui-ux-design-system.md §5.9, Screen 16. */
export function GuidesIndexPage() {
  return (
    <div className="flex min-h-screen flex-col">
      <div className="px-4 pt-6">
        <PublicNav />
      </div>

      <main className="mx-auto w-full max-w-4xl flex-1 px-8 py-16">
        <p className="text-caption font-semibold uppercase tracking-wide text-accent">
          Trident field guides
        </p>
        <h1
          className="mt-2 text-display-section"
          style={{ fontFamily: 'var(--font-display-serif)' }}
        >
          Security concepts, connected.
        </h1>
        <p className="mt-4 max-w-xl text-body text-text-secondary">
          Practical guides for running GuardPipe end to end — from your first scan to reading a risk
          score you actually trust.
        </p>

        <div className="mt-12 grid grid-cols-1 gap-6 sm:grid-cols-3">
          {guides.map((guide) => (
            <Link
              key={guide.slug}
              to={`/guides/${guide.slug}`}
              className="rounded-lg border border-border-default bg-bg-surface p-6 shadow-sm transition-colors hover:bg-bg-subtle"
              style={{ transitionDuration: 'var(--duration-fast)' }}
            >
              <p className="text-caption font-semibold uppercase tracking-wide text-accent">
                {guide.category}
              </p>
              <h2 className="mt-2 text-h2 text-text-primary">{guide.title}</h2>
              <p className="mt-2 text-body-sm text-text-secondary">{guide.description}</p>
            </Link>
          ))}
        </div>
      </main>

      <PublicFooter />
    </div>
  )
}
