import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { AuthSidePanel } from './AuthSidePanel'
import { Logo } from './Logo'

/**
 * Split-screen Login/Register shell — direction adapted from Aikido's
 * one-click-login screen: a fixed-white form panel (logo top-left,
 * centred form column) beside a fixed-dark info panel (`AuthSidePanel`).
 * Adapted, not copied — the reference uses OAuth provider buttons
 * (GitHub/GitLab/Bitbucket); GuardPipe has no OAuth (FR-IAM-010 is a
 * documented Stretch goal, not built), so the centre column is a real
 * email/password form instead, same layout, real functionality
 * (documentation/09-ui-ux-design-system.md §5.8).
 *
 * Replaces the previous single-dark-hero-with-floating-nav treatment —
 * no `PublicNav` here on purpose: a focused auth screen with just a way
 * back to the marketing site, not a full site nav competing for
 * attention mid-signup.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen">
      <div className="auth-panel-light flex w-full flex-col bg-auth-panel-bg px-6 py-6 text-auth-panel-fg sm:px-10 lg:w-1/2">
        <Link to="/" className="flex items-center gap-2 text-h3 font-semibold">
          <Logo className="h-7 w-auto" />
          GuardPipe
        </Link>
        <div className="flex flex-1 flex-col items-center justify-center py-10">
          <div className="w-full max-w-sm">{children}</div>
        </div>
      </div>

      <AuthSidePanel />
    </div>
  )
}
