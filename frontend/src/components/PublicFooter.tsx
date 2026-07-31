import { Link } from 'react-router-dom'

function FooterColumn({ title, links }: { title: string; links: { to: string; label: string }[] }) {
  return (
    <div>
      <p className="text-caption font-semibold tracking-wide text-white/40 uppercase">{title}</p>
      <ul className="mt-3 flex flex-col gap-2">
        {links.map((l) => (
          <li key={l.to}>
            <Link to={l.to} className="text-white/70 hover:text-white">
              {l.label}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

/**
 * Shared footer for Landing, Guides, and Blog — dark background per the
 * original wireframe (documentation/09-ui-ux-design-system.md §5.9: "Footer
 * | Nav columns + legal, dark background"). Text uses explicit white/opacity
 * utilities rather than the `--text-*` tokens, since those follow the app's
 * light/dark mode and would resolve to dark-on-dark here — this footer is
 * deliberately always dark, independent of the page's own mode.
 */
export function PublicFooter() {
  return (
    <footer
      className="mt-24 overflow-hidden px-8 pt-16 text-body-sm text-white/70"
      style={{ backgroundColor: 'var(--public-bg-hero)' }}
    >
      <div className="mx-auto flex max-w-5xl flex-wrap justify-between gap-10 border-b border-white/10 pb-12">
        <div className="max-w-xs">
          <p className="text-h3 text-white">GuardPipe</p>
          <p className="mt-2 text-white/70">
            One risk score across the whole software supply chain — design, code, dependencies,
            containers, Kubernetes, CI/CD, and runtime.
          </p>
        </div>
        <FooterColumn
          title="Product"
          links={[
            { to: '/', label: 'Overview' },
            { to: '/guides', label: 'Guides' },
          ]}
        />
        <FooterColumn title="Resources" links={[{ to: '/blog', label: 'Blog' }]} />
        <FooterColumn
          title="Account"
          links={[
            { to: '/login', label: 'Sign in' },
            { to: '/register', label: 'Register' },
          ]}
        />
      </div>

      <div className="py-8 text-center">
        <p
          aria-hidden="true"
          className="mx-auto max-w-4xl leading-none font-semibold tracking-tight select-none"
          style={{
            fontFamily: 'var(--font-display-serif)',
            fontSize: 'clamp(2.25rem, 8vw, 6.5rem)',
            backgroundImage:
              'linear-gradient(180deg, rgba(255,255,255,0.16), rgba(255,255,255,0.02))',
            WebkitBackgroundClip: 'text',
            backgroundClip: 'text',
            color: 'transparent',
          }}
        >
          Security At Every Layer
        </p>
        {/* Real text for screen readers / SEO — the styled copy above is
            aria-hidden since its low-contrast gradient fill can render as
            near-invisible, which would otherwise fail an accessible-text
            check despite being decorative here. */}
        <span className="sr-only">Security At Every Layer</span>
      </div>
    </footer>
  )
}
