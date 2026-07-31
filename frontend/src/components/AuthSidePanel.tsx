import { FolderGit2, Gauge, ShieldCheck } from 'lucide-react'
import { HeroGlow } from './HeroGlow'

/**
 * The dark right-hand panel on Login/Register — direction adapted from
 * Aikido's split-screen login (documentation/09-ui-ux-design-system.md
 * §5.9-adjacent auth treatment): a fixed-dark info panel next to the
 * fixed-white form panel (`AuthShell`). Adapted, not copied: Aikido's
 * cards are OAuth-provider setup steps and third-party trust badges
 * ("Trusted by...", SOC 2, ISO 27001) — GuardPipe has no OAuth, no
 * customers, and no compliance certification, so fabricating any of that
 * would be exactly the kind of dishonest UI this project has avoided
 * everywhere else (`GlobalSearch`, `NotificationPanel`, §4.4/§4.5). These
 * three cards are real GuardPipe claims already made on the Landing page
 * (documentation/09-ui-ux-design-system.md §5.9's feature cards) — same
 * copy, reused here instead of invented.
 */
const CARDS = [
  {
    icon: FolderGit2,
    title: 'Connect a repository or a target',
    body: 'Attach a GitHub repository or register a pentest target — GuardPipe scans across all seven supply-chain stages.',
  },
  {
    icon: ShieldCheck,
    title: 'Sandboxed by default',
    body: 'Pentest and container inspection run inside a no-network, read-only, non-root container. A security tool that isn’t itself secure is worthless.',
  },
  {
    icon: Gauge,
    title: 'One explainable score',
    body: 'Every finding normalises into a single 0–100 risk verdict, with AI-authored fixes you can actually read.',
  },
] as const

export function AuthSidePanel() {
  return (
    <div className="relative hidden overflow-hidden lg:flex lg:w-1/2">
      <HeroGlow />
      <div className="relative z-10 flex flex-1 flex-col justify-center gap-4 px-12 py-12 text-public-fg">
        {CARDS.map(({ icon: Icon, title, body }) => (
          <div key={title} className="rounded-lg border border-white/15 bg-white/5 p-5">
            <div className="flex items-center gap-2.5">
              <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span className="text-body-sm font-semibold">{title}</span>
            </div>
            <p className="mt-1.5 text-body-sm text-white/70">{body}</p>
          </div>
        ))}
      </div>
    </div>
  )
}
