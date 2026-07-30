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
