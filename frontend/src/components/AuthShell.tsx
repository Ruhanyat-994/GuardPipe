import type { ReactNode } from 'react'
import { HeroGlow } from './HeroGlow'
import { PublicNav } from './PublicNav'

/**
 * Shared shell for Login/Register — the same dark hero glow and floating
 * nav as the Landing page, so the auth pages read as part of the same
 * product rather than a plain, disconnected form screen.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="relative flex min-h-screen flex-col overflow-hidden text-public-fg">
      <HeroGlow />
      <div className="relative px-4 pt-6">
        <PublicNav />
      </div>
      <main className="relative flex flex-1 items-center justify-center px-4 py-16">
        <div className="w-full max-w-sm">{children}</div>
      </main>
    </div>
  )
}
