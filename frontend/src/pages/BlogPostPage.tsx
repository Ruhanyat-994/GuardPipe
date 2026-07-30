import { Link, useParams } from 'react-router-dom'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { getPostBySlug } from '../content/posts'
import { PlaceholderPage } from './PlaceholderPage'

/** documentation/09-ui-ux-design-system.md §5.9, Screen 15. */
export function BlogPostPage() {
  const { slug } = useParams<{ slug: string }>()
  const post = slug ? getPostBySlug(slug) : undefined

  if (!post) {
    return <PlaceholderPage title="Post not found" phase="—" />
  }

  return (
    <div className="flex min-h-screen flex-col">
      <div className="px-4 pt-6">
        <PublicNav />
      </div>

      <main className="mx-auto w-full max-w-2xl flex-1 px-8 py-16">
        <Link to="/blog" className="text-body-sm text-accent">
          ← All posts
        </Link>
        <p className="mt-4 text-caption font-semibold uppercase tracking-wide text-accent">
          {post.category}
        </p>
        <h1
          className="mt-2 text-display-section"
          style={{ fontFamily: 'var(--font-display-serif)' }}
        >
          {post.title}
        </h1>
        <p className="mt-3 text-caption text-text-tertiary">
          {formatDate(post.date)} · {post.readTimeMinutes} min read
        </p>

        <div className="mt-8 flex max-w-prose flex-col gap-4">
          {post.body.map((paragraph, i) => (
            <p key={i} className="text-body text-text-secondary">
              {paragraph}
            </p>
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
