import { Link } from 'react-router-dom'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { posts } from '../content/posts'

/** documentation/09-ui-ux-design-system.md §5.9, Screen 14. */
export function BlogIndexPage() {
  return (
    <div className="flex min-h-screen flex-col">
      <div className="px-4 pt-6">
        <PublicNav />
      </div>

      <main className="mx-auto w-full max-w-4xl flex-1 px-8 py-16">
        <p className="text-caption font-semibold uppercase tracking-wide text-accent">
          How to use GuardPipe
        </p>
        <h1
          className="mt-2 text-display-section"
          style={{ fontFamily: 'var(--font-display-serif)' }}
        >
          Practical notes, not a research blog.
        </h1>
        <p className="mt-4 max-w-xl text-body text-text-secondary">
          Short, focused posts about actually using the platform — what a partial scan means, how to
          read your first score, how credentials are handled.
        </p>

        <div className="mt-12 grid grid-cols-1 gap-6 sm:grid-cols-2">
          {posts.map((post) => (
            <Link
              key={post.slug}
              to={`/blog/${post.slug}`}
              className="rounded-lg border border-border-default bg-bg-surface p-6 shadow-sm transition-colors hover:bg-bg-subtle"
              style={{ transitionDuration: 'var(--duration-fast)' }}
            >
              <p className="text-caption font-semibold uppercase tracking-wide text-accent">
                {post.category}
              </p>
              <h2 className="mt-2 text-h2 text-text-primary">{post.title}</h2>
              <p className="mt-2 text-body-sm text-text-secondary">{post.description}</p>
              <p className="mt-4 text-caption text-text-tertiary">
                {formatDate(post.date)} · {post.readTimeMinutes} min read
              </p>
            </Link>
          ))}
        </div>
      </main>

      <PublicFooter />
    </div>
  )
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}
