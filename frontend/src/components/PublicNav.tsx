import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { Logo } from './Logo'
import { cn } from '../lib/cn'
import { useAuthStore } from '../stores/authStore'

const LINKS = [
  { to: '/#engines', label: 'Engines' },
  { to: '/pricing', label: 'Pricing' },
  { to: '/guides', label: 'Guides' },
  { to: '/blog', label: 'Blog' },
]

/**
 * Public-site navigation (Landing, Pricing, Guides, Blog).
 *
 * `variant="hero"` (the landing page): fixed, transparent with white text
 * over the dark hero, then collapses into a floating white pill once the
 * page scrolls — the same move the reference site makes. Every other page
 * uses the pill from the start (`variant="pill"`, the default).
 */
export function PublicNav({ variant = 'pill' }: { variant?: 'hero' | 'pill' }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const [scrolled, setScrolled] = useState(false)

  useEffect(() => {
    if (variant !== 'hero') return
    const onScroll = () => setScrolled(window.scrollY > 40)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [variant])

  const onDark = variant === 'hero' && !scrolled
  const ctaTo = isAuthenticated ? '/projects' : '/register'

  return (
    <div
      className={cn(
        'z-40 flex w-full justify-center px-4',
        variant === 'hero' ? 'fixed inset-x-0 top-3' : 'sticky top-3',
      )}
    >
      <header
        className={cn(
          'flex w-full items-center justify-between transition-all',
          onDark
            ? 'max-w-5xl px-2 py-3 text-white'
            : 'max-w-3xl rounded-full border border-black/5 bg-white/95 py-2 pr-2 pl-5 text-neutral-900 shadow-[0_8px_30px_rgba(0,0,0,0.12)] backdrop-blur',
        )}
        style={{ transitionDuration: '350ms' }}
      >
        <Link to="/" className="flex items-center gap-2 text-[15px] font-semibold tracking-tight">
          <Logo className="h-6 w-auto" />
          GuardPipe
        </Link>

        <nav className="hidden items-center gap-6 text-[13px] font-medium md:flex">
          {LINKS.map((l) => (
            <Link
              key={l.to}
              to={l.to}
              className={cn(
                'transition-colors',
                onDark
                  ? 'text-white/75 hover:text-white'
                  : 'text-neutral-600 hover:text-neutral-950',
              )}
            >
              {l.label}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-3">
          {!isAuthenticated && (
            <Link
              to="/login"
              className={cn(
                'hidden text-[13px] font-medium sm:inline',
                onDark
                  ? 'text-white/80 hover:text-white'
                  : 'text-neutral-600 hover:text-neutral-950',
              )}
            >
              Sign in
            </Link>
          )}
          <Link
            to={ctaTo}
            className={cn(
              'group inline-flex items-center gap-1 rounded-full px-4 py-2 text-[13px] font-semibold transition-colors',
              onDark
                ? 'bg-white text-neutral-950 hover:bg-white/90'
                : 'bg-neutral-950 text-white hover:bg-neutral-800',
            )}
          >
            {isAuthenticated ? 'Dashboard' : 'Run a quick test'}
            <ChevronRight
              className="h-3.5 w-3.5 transition-transform group-hover:translate-x-0.5"
              aria-hidden="true"
            />
          </Link>
        </div>
      </header>
    </div>
  )
}
