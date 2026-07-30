/**
 * Static guide content — Markdown/MDX bundled into the frontend build, no
 * CMS, no backend module (documentation/09-ui-ux-design-system.md §5.9).
 * Kept as plain TS data for now rather than adding an MDX toolchain; the
 * "static, bundled" requirement is satisfied either way, and this avoids a
 * dependency for the amount of content that exists so far.
 */

export interface GuideSection {
  heading: string
  body: string
}

export interface Guide {
  slug: string
  category: string
  title: string
  description: string
  sections: GuideSection[]
}

export const guides: Guide[] = [
  {
    slug: 'running-your-first-scan',
    category: 'Getting started',
    title: 'Running your first scan',
    description: 'From an empty account to a completed supply-chain scan.',
    sections: [
      {
        heading: 'Create a project',
        body: 'A project groups one repository (and, optionally, one pentest target) under a single risk score. You need at least one project before you can run a scan.',
      },
      {
        heading: 'Attach a repository',
        body: 'GuardPipe needs read access to clone your repository. For a private repository, you’ll attach a GitHub personal access token — it’s encrypted at rest and never echoed back by any API response.',
      },
      {
        heading: 'Choose your engines',
        body: 'A "Full Supply Chain Scan" runs all seven engines. You can also run a single engine on its own — useful the first time, when you just want to see what depscan finds before committing to a full run.',
      },
      {
        heading: 'Watch it run',
        body: 'Findings stream in as each engine finishes, not all at once at the end — you’ll see the pipeline view update stage by stage.',
      },
    ],
  },
  {
    slug: 'reading-your-risk-score',
    category: 'Getting started',
    title: 'Reading your risk score and verdict',
    description: 'What the 0–100 number means, and why it isn’t just an average.',
    sections: [
      {
        heading: 'It’s not an average',
        body: 'The score is a weighted formula over severity-normalised findings, with saturation and a critical/secret floor — one hardcoded credential in a live cloud account will dominate the score even if every other finding is informational.',
      },
      {
        heading: 'Three verdicts',
        body: '`pass`, `warn`, and `block` are threshold bands over the same score (configurable via GUARDPIPE_GATE_WARN / GUARDPIPE_GATE_BLOCK). A block verdict is meant to be a real gate, not just a colour.',
      },
      {
        heading: 'Per-engine breakdown',
        body: 'The headline score is one number, but every engine’s contribution is visible underneath it — you can tell at a glance whether your risk is coming from dependencies, code, or infrastructure.',
      },
    ],
  },
  {
    slug: 'connecting-a-repository-and-target',
    category: 'Getting started',
    title: 'Connecting a repository and pentest target',
    description: 'Credentials, validation, and the authorisation step pentest requires.',
    sections: [
      {
        heading: 'Repository credentials',
        body: 'A GitHub personal access token is encrypted with AES-256-GCM before it touches the database, and no API response — not even your own — ever includes it back. If you need to check which token is attached, you’ll see a masked hint like `ghp_•••3f9a`, never the value itself.',
      },
      {
        heading: 'Pentest targets are validated before anything runs',
        body: 'A target is DNS-resolved and checked against private/loopback/metadata ranges before it’s ever eligible for a scan. A target resolving to a private address is blocked outright, with a plain-language reason.',
      },
      {
        heading: 'Attestation is a legal record',
        body: 'Running a pentest requires an explicit ownership attestation — a checkbox alone won’t do; you’ll see the full authorisation statement with your name and the target before the option to start is even enabled.',
      },
    ],
  },
]

export function getGuideBySlug(slug: string): Guide | undefined {
  return guides.find((g) => g.slug === slug)
}
