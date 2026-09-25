import { useState } from 'react'
import { Link } from 'react-router-dom'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { BlogCover } from '../components/landing/BlogCover'
import { posts } from '../content/posts'
import { cn } from '../lib/cn'

/**
 * documentation/09-ui-ux-design-system.md §5.9, Screen 14 — restyled to
 * match the landing page: light page inside the same dashed column guides,
 * a two-line header, category filter pills, and a three-column card grid
 * with generated space-motif covers.
 */
export function BlogIndexPage() {
  const categories = ['All', ...Array.from(new Set(posts.map((p) => p.category)))]
  const [active, setActive] = useState('All')
  const shown = active === 'All' ? posts : posts.filter((p) => p.category === active)

  return (
    <div className="flex min-h-screen flex-col bg-[#f7f7f5] text-neutral-900">
      <div className="pt-3">
        <PublicNav />
      </div>

      <main className="mx-auto w-full max-w-5xl flex-1 border-x border-dashed border-black/[0.08] px-6 pt-20 pb-28">
        <div className="flex flex-wrap items-end justify-between gap-6">
          <h1 className="text-[34px] leading-tight font-semibold tracking-tight">
            Blog
            <span className="block text-neutral-400">Notes from the pipeline</span>
          </h1>
          <p className="max-w-xs text-[14px] leading-6 text-neutral-500">
            How to use GuardPipe, what we shipped, and the security and engineering decisions behind
            it.
          </p>
        </div>

        <div className="mt-12 flex flex-wrap gap-2" role="tablist" aria-label="Filter posts">
          {categories.map((c) => (
            <button
              key={c}
              type="button"
              role="tab"
              aria-selected={active === c}
              onClick={() => setActive(c)}
              className={cn(
                'rounded-full px-3.5 py-1.5 text-[13px] font-medium transition-colors',
                active === c
                  ? 'bg-neutral-950 text-white'
                  : 'text-neutral-500 hover:bg-black/5 hover:text-neutral-900',
              )}
            >
              {c}
            </button>
          ))}
        </div>

        <div className="mt-8 grid grid-cols-1 gap-x-5 gap-y-10 sm:grid-cols-2 lg:grid-cols-3">
          {shown.map((post) => (
            <Link key={post.slug} to={`/blog/${post.slug}`} className="group flex flex-col">
              <div className="overflow-hidden rounded-xl ring-1 ring-black/5 transition-shadow group-hover:shadow-[0_18px_40px_rgba(3,5,11,0.18)]">
                <BlogCover
                  slug={post.slug}
                  category={post.category}
                  className="aspect-[16/9] w-full transition-transform duration-500 group-hover:scale-[1.04]"
                />
              </div>
              <p className="mt-4 flex items-center gap-2 text-[12px] text-neutral-500">
                <span className="font-medium text-[#2563eb]">{post.category}</span>
                <span aria-hidden="true">·</span>
                {formatDate(post.date)}
                <span aria-hidden="true">·</span>
                {post.readTimeMinutes} min read
              </p>
              <h2 className="mt-2 text-[16px] leading-6 font-semibold group-hover:underline group-hover:decoration-black/20 group-hover:underline-offset-4">
                {post.title}
              </h2>
              <p className="mt-1.5 line-clamp-2 text-[13px] leading-5 text-neutral-500">
                {post.description}
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
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}
