import { Link, useParams } from 'react-router-dom'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { getGuideBySlug } from '../content/guides'
import { PlaceholderPage } from './PlaceholderPage'

/** documentation/09-ui-ux-design-system.md §5.9, Screen 17. */
export function GuideDetailPage() {
  const { slug } = useParams<{ slug: string }>()
  const guide = slug ? getGuideBySlug(slug) : undefined

  if (!guide) {
    return <PlaceholderPage title="Guide not found" phase="—" />
  }

  return (
    <div className="flex min-h-screen flex-col">
      <div className="px-4 pt-6">
        <PublicNav />
      </div>

      <main className="mx-auto grid w-full max-w-4xl flex-1 grid-cols-1 gap-10 px-8 py-16 sm:grid-cols-[200px_1fr]">
        <aside className="hidden sm:block">
          <nav className="sticky top-24 flex flex-col gap-2 text-body-sm">
            {guide.sections.map((s) => (
              <a
                key={s.heading}
                href={`#${slugify(s.heading)}`}
                className="text-text-secondary hover:text-text-primary"
              >
                {s.heading}
              </a>
            ))}
          </nav>
        </aside>

        <article>
          <Link to="/guides" className="text-body-sm text-accent">
            ← All guides
          </Link>
          <h1
            className="mt-3 text-display-section"
            style={{ fontFamily: 'var(--font-display-serif)' }}
          >
            {guide.title}
          </h1>
          <p className="mt-3 text-body text-text-secondary">{guide.description}</p>

          <div className="mt-10 flex flex-col gap-8">
            {guide.sections.map((section) => (
              <section key={section.heading} id={slugify(section.heading)}>
                <h2 className="text-h2 text-text-primary">{section.heading}</h2>
                <p className="mt-2 max-w-prose text-body text-text-secondary">{section.body}</p>
              </section>
            ))}
          </div>
        </article>
      </main>

      <PublicFooter />
    </div>
  )
}

function slugify(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, '-')
}
