import { Link } from 'react-router-dom'
import { Logo } from './Logo'

const COLUMNS = [
  {
    title: 'Product',
    links: [
      { to: '/#engines', label: 'Engines' },
      { to: '/pricing', label: 'Pricing' },
      { to: '/register', label: 'Run a quick test' },
    ],
  },
  {
    title: 'Resources',
    links: [
      { to: '/guides', label: 'Guides' },
      { to: '/blog', label: 'Blog' },
    ],
  },
  {
    title: 'Account',
    links: [
      { to: '/login', label: 'Sign in' },
      { to: '/register', label: 'Create account' },
    ],
  },
]

/**
 * Shared public footer — always dark, whatever the page's own theme.
 * Closes on the motto as a giant dashed-outline wordmark (SVG text, so it
 * scales to any width without a font-size guess). Colours are literal
 * white/opacity on purpose: the `--text-*` tokens follow the app's
 * light/dark mode and would go dark-on-dark here.
 */
export function PublicFooter() {
  return (
    <footer className="relative overflow-hidden bg-[#05070c] text-white">
      <div className="mx-auto max-w-5xl border-x border-dashed border-white/[0.07] px-6 pt-20">
        <div className="flex flex-wrap justify-between gap-12">
          <div className="max-w-xs">
            <Link to="/" className="flex items-center gap-2 text-lg font-semibold">
              <Logo className="h-6 w-auto" />
              GuardPipe
            </Link>
            <p className="mt-4 text-[13px] leading-6 text-white/55">
              Software supply-chain security. Seven layers — design, code, dependencies, containers,
              Kubernetes, CI/CD and runtime — one explainable score.
            </p>
          </div>
          <div className="flex flex-wrap gap-14">
            {COLUMNS.map((col) => (
              <div key={col.title}>
                <p className="text-[13px] font-semibold text-white">{col.title}</p>
                <ul className="mt-4 flex flex-col gap-2.5">
                  {col.links.map((l) => (
                    <li key={l.label}>
                      <Link to={l.to} className="text-[13px] text-white/50 hover:text-white">
                        {l.label}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>

        <svg
          viewBox="0 0 1000 150"
          className="mt-16 block w-full"
          aria-hidden="true"
          preserveAspectRatio="xMidYMax meet"
        >
          <text
            x="500"
            y="128"
            textAnchor="middle"
            textLength="980"
            lengthAdjust="spacingAndGlyphs"
            fontFamily="Inter, ui-sans-serif, system-ui, sans-serif"
            fontWeight={800}
            fontSize={150}
            fill="rgba(255,255,255,0.025)"
            stroke="rgba(255,255,255,0.16)"
            strokeWidth={1.2}
            strokeDasharray="7 6"
          >
            Secure Every Layer
          </text>
        </svg>
        <span className="sr-only">Secure every layer.</span>

        <div className="flex flex-wrap items-center justify-between gap-4 border-t border-white/[0.07] py-6 text-[12px] text-white/40">
          <p>© {new Date().getFullYear()} GuardPipe. Secure every layer.</p>
          <p>Built to be attacked — and survive it.</p>
        </div>
      </div>
    </footer>
  )
}
