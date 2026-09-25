import { Calendar, GitBranch, GitPullRequest, Terminal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ScanTrigger } from '../../lib/scansApi'

/** Human-readable origin of a scan, from its trigger_* fields (migration
 * 00027). null for manual scans and for scans created before origins were
 * recorded — nothing to call out in either case. */
function describeScanOrigin(s: ScanTrigger): { icon: LucideIcon; text: string } | null {
  const by = s.trigger_actor ? ` by ${s.trigger_actor}` : ''
  switch (s.trigger_source) {
    case 'webhook_push':
      return { icon: GitBranch, text: `Push to ${s.trigger_ref ?? 'a branch'}${by}` }
    case 'webhook_pull_request': {
      const pr = s.trigger_ref?.match(/^refs\/pull\/(\d+)\/head$/)
      return { icon: GitPullRequest, text: `${pr ? `PR #${pr[1]}` : 'Pull request'}${by}` }
    }
    case 'scheduled':
      return { icon: Calendar, text: 'Scheduled' }
    case 'cli_watch':
      return { icon: Terminal, text: `Local commit${by}` }
    default:
      return null
  }
}

export function ScanOriginBadge({ scan, className }: { scan: ScanTrigger; className?: string }) {
  const origin = describeScanOrigin(scan)
  if (!origin) return null
  const Icon = origin.icon
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full bg-accent/10 px-2 py-0.5 text-caption font-medium text-accent ${className ?? ''}`}
      title="What started this scan"
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {origin.text}
    </span>
  )
}
