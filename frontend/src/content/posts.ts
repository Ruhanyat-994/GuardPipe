/**
 * Static blog content, scoped to practical "how to use GuardPipe" posts
 * rather than general security research (documentation/09-ui-ux-design-system.md
 * §5.9's Screen 14 scope note). See guides.ts for the note on why this is
 * plain TS data rather than an MDX pipeline.
 */

export interface Post {
  slug: string
  category: string
  title: string
  description: string
  date: string
  readTimeMinutes: number
  body: string[]
}

export const posts: Post[] = [
  {
    slug: 'scans-run-in-the-background',
    category: 'Product',
    title: 'Start a scan, then go do something else',
    description:
      'Scans now follow you around the app, and the PDF report can land in your inbox when they finish.',
    date: '2026-09-25',
    readTimeMinutes: 3,
    body: [
      'A full supply-chain scan can take a few minutes — longer with a pentest. Scans always ran on the server, but the app made it feel like you had to sit and watch the progress bar. You don’t.',
      'Start a scan and leave the page. A small "scans running" indicator in the top bar follows you everywhere, with live progress for each one, and a notification pops up the moment one finishes — with its risk score and a link straight to the results.',
      'The bell keeps a record of every finished scan, so you can catch up later. And if you want it, the full PDF report is emailed to you: to your account address by default, or to another address you choose in Settings → Notifications.',
      'Live scans (the ones a GitHub push starts) can email too, but that’s off by default — a busy repository can push a lot, and nobody wants forty reports before lunch.',
    ],
  },
  {
    slug: 'why-we-confirm-report-addresses',
    category: 'Security',
    title: 'Why we make you confirm a report address',
    description:
      'A scan report is a map of your weaknesses. Where it gets emailed is a security decision.',
    date: '2026-09-25',
    readTimeMinutes: 3,
    body: [
      'You can send scan reports to an address other than your account email — a shared security inbox, say. But GuardPipe won’t send a single report there until someone clicks a confirmation link sent to that address.',
      'Here is the attack that step prevents: someone gets into an account for a few minutes, changes the report address to their own mailbox, and quietly receives every future vulnerability report — long after the original break-in is noticed and the password changed.',
      'So a new address sits as "pending" until confirmed. The link works once and expires after 24 hours, and we only store a hash of it. Reports keep going to the old address in the meantime, and both the request and the confirmation are written to the audit log.',
      'Reports only ever go to the person accountable for the scan — whoever started it, created its schedule, or turned on live scanning. Never to an arbitrary list.',
    ],
  },
  {
    slug: 'the-scan-that-stayed-queued',
    category: 'Engineering',
    title: 'The race that left a scan queued forever',
    description:
      'Seven engines finished in 80 milliseconds, and nobody closed the scan. Here’s why, and the fix.',
    date: '2026-09-25',
    readTimeMinutes: 4,
    body: [
      'A scan is finished when its last engine is. Each engine’s result is saved in its own database transaction, which then asks: "is every job for this scan done now?" If yes, it closes the scan and computes the score.',
      'That works until the engines finish at the same moment. On a repository that couldn’t be cloned, all seven failed within 80 ms. Each transaction looked at the others before they had committed, saw them still running, and concluded it wasn’t the last one. Every engine was done; the scan said "queued" forever.',
      'The fix is small: every job result now locks its scan’s row first, so results for one scan are recorded one at a time, and whichever commits last is guaranteed to see all the others. Closing a scan is also idempotent now, so scoring and notifications run exactly once.',
      'We wrote a test that fires seven results at one scan simultaneously against a real Postgres. It fails without the lock and passes with it — and a background sweeper now repairs any scan that was already stuck.',
    ],
  },
  {
    slug: 'what-partial-scan-means',
    category: 'Getting started',
    title: 'What "partial scan" means, and when to trust it',
    description:
      'One engine failing doesn’t mean the whole scan is worthless — here’s how to read it.',
    date: '2026-07-15',
    readTimeMinutes: 3,
    body: [
      'A scan can complete with some engines skipped and others failed — GuardPipe never hides this behind a green checkmark. A "partial" banner names exactly which engines didn’t run, and why.',
      '"Skipped" and "failed" mean different things. A skip is expected: containerscan skips a repository with no Dockerfile, because there is nothing for it to look at. A failure means something went wrong — a timeout, an unreachable AI service, a panic inside the engine — and is always visible, never silently swallowed.',
      'The score itself is computed only from the engines that actually ran. A partial scan is still meaningful; it just isn’t the full picture. If you need the full picture, re-run the scan once the failing engine’s dependency (usually the AI service or the sandbox) is back.',
    ],
  },
  {
    slug: 'reading-your-first-risk-score',
    category: 'Getting started',
    title: 'Reading your first risk score',
    description: 'A walkthrough of the dashboard, five minutes after your first scan finishes.',
    date: '2026-07-08',
    readTimeMinutes: 4,
    body: [
      'The first thing you see after a scan completes is a single number, 0 to 100, next to a verdict word — pass, warn, or block. That number is deliberately the loudest thing on the page.',
      'Underneath it, five stat tiles break the same findings down by severity. These are the one place GuardPipe uses full-saturation colour — everywhere else, colour is an accent, not a wall of red, so that when something really is critical, it stands out instead of blending into the noise.',
      'Below that, the supply-chain pipeline shows all seven stages and which ones contributed to the score. Click any stage, or any severity tile, and the findings explorer filters itself accordingly — you never have to build that filter by hand.',
    ],
  },
  {
    slug: 'connecting-a-private-repository',
    category: 'Getting started',
    title: 'Connecting a private repository',
    description: 'What GuardPipe needs, what it never stores, and what the masked hint means.',
    date: '2026-06-30',
    readTimeMinutes: 3,
    body: [
      'GuardPipe needs a GitHub personal access token to clone a private repository. That token is encrypted (AES-256-GCM) the moment it reaches the server, and the plaintext value is never written to a log, never returned by any API response — including the one that lists your own project’s settings.',
      'What you’ll see instead is a masked hint, like `ghp_•••3f9a` — enough to recognise which token is attached, never enough to reconstruct it.',
      'If a token expires or is revoked on GitHub’s side, the next scan attempt fails clearly with a "repository unreachable" error rather than a confusing generic failure — you shouldn’t have to guess why a clone stopped working.',
    ],
  },
]

export function getPostBySlug(slug: string): Post | undefined {
  return posts.find((p) => p.slug === slug)
}
