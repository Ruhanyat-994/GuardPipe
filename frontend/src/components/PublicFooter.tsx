import { Link } from 'react-router-dom'

function FooterColumn({ title, links }: { title: string; links: { to: string; label: string }[] }) {
  return (
    <div>
      <p className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
        {title}
      </p>
      <ul className="mt-3 flex flex-col gap-2">
        {links.map((l) => (
          <li key={l.to}>
            <Link to={l.to} className="text-text-secondary hover:text-text-primary">
              {l.label}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Shared footer for Landing, Guides, and Blog. */
export function PublicFooter() {
  return (
    <footer className="mt-24 border-t border-border-default bg-bg-subtle px-8 py-12 text-body-sm">
      <div className="mx-auto flex max-w-5xl flex-wrap justify-between gap-10">
        <div className="max-w-xs">
          <p className="text-h3 text-text-primary">GuardPipe</p>
          <p className="mt-2 text-text-secondary">
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
    </footer>
  )
}
