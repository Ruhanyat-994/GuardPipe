import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { BlogCover } from '../components/landing/BlogCover'
import { getPostBySlug, posts } from '../content/posts'
import { PlaceholderPage } from './PlaceholderPage'

/** documentation/09-ui-ux-design-system.md §5.9, Screen 15 — matches the
 * restyled blog index (see BlogIndexPage). */
export function BlogPostPage() {
  const { slug } = useParams<{ slug: string }>()
  const post = slug ? getPostBySlug(slug) : undefined

  if (!post) {
    return <PlaceholderPage title="Post not found" phase="—" />
  }

  const more = posts.filter((p) => p.slug !== post.slug).slice(0, 3)

  return (
    <div className="flex min-h-screen flex-col bg-[#f7f7f5] text-neutral-900">
      <div className="pt-3">
        <PublicNav />
      </div>

      <main className="mx-auto w-full max-w-5xl flex-1 border-x border-dashed border-black/[0.08] px-6 pt-16 pb-28">
        <article className="mx-auto max-w-2xl">
          <Link
            to="/blog"
            className="inline-flex items-center gap-1.5 text-[13px] font-medium text-neutral-500 hover:text-neutral-900"
          >
            <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
            All posts
          </Link>
          <p className="mt-8 flex items-center gap-2 text-[12px] text-neutral-500">
            <span className="font-medium text-[#2563eb]">{post.category}</span>
            <span aria-hidden="true">·</span>
            {formatDate(post.date)}
            <span aria-hidden="true">·</span>
            {post.readTimeMinutes} min read
          </p>
          <h1 className="mt-3 text-[34px] leading-tight font-semibold tracking-tight">
            {post.title}
          </h1>
          <p className="mt-3 text-[16px] leading-7 text-neutral-500">{post.description}</p>

          <div className="mt-10 overflow-hidden rounded-2xl ring-1 ring-black/5">
            <BlogCover slug={post.slug} category={post.category} className="aspect-[16/9] w-full" />
          </div>

          <div className="mt-10 flex flex-col gap-5">
            {post.body.map((paragraph, i) => (
              <p key={i} className="text-[16px] leading-7 text-neutral-700">
                {paragraph}
              </p>
            ))}
          </div>
        </article>

        {more.length > 0 && (
          <section className="mt-24 border-t border-dashed border-black/[0.08] pt-12">
            <h2 className="text-[20px] font-semibold tracking-tight">More from the blog</h2>
            <div className="mt-6 grid grid-cols-1 gap-5 sm:grid-cols-3">
              {more.map((p) => (
                <Link key={p.slug} to={`/blog/${p.slug}`} className="group">
                  <div className="overflow-hidden rounded-xl ring-1 ring-black/5">
                    <BlogCover
                      slug={p.slug}
                      category={p.category}
                      className="aspect-[16/9] w-full transition-transform duration-500 group-hover:scale-[1.04]"
                    />
                  </div>
                  <p className="mt-3 text-[14px] leading-5 font-semibold group-hover:underline group-hover:underline-offset-4">
                    {p.title}
                  </p>
                </Link>
              ))}
            </div>
          </section>
        )}
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
