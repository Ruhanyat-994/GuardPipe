import { Link } from 'react-router-dom'
import { HeroGlow } from '../components/HeroGlow'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'

const ENGINE_STAGES = ['Docs', 'Code', 'Deps', 'Containers', 'K8s', 'CI/CD', 'Pentest']

/**
 * documentation/09-ui-ux-design-system.md §5.9, Screen 13. Direction
 * adapted from tridentsecurity.io: dark hero with an animated blue gradient
 * glow behind a serif headline, floating pill nav, alternating feature
 * cards, closing CTA band. GuardPipe keeps one accent colour (no second
 * red), and a gradient CTA band instead of a stock photo — there's no
 * photography asset for this project, and a fabricated one would be worse
 * than an honest gradient.
 */
export function LandingPage() {
  return (
    <div className="flex min-h-screen flex-col">
      <div className="relative overflow-hidden text-text-inverse">
        <HeroGlow />
        <div className="relative px-4 pt-6">
          <PublicNav />
        </div>

        <div className="relative mx-auto max-w-3xl px-8 py-32 text-center">
          <h1 className="text-display-hero" style={{ fontFamily: 'var(--font-display-serif)' }}>
            One score for your whole supply chain.
          </h1>
          <p className="mx-auto mt-6 max-w-xl text-body text-white/80">
            Seven SDLC stages, one explainable 0–100 risk verdict, AI-authored fixes. Built to be
            attacked and survive it.
          </p>
          <Link
            to="/register"
            className="mt-8 inline-block rounded-md bg-accent px-6 py-3 text-body font-medium text-white hover:opacity-90"
          >
            Get started free
          </Link>
        </div>

        <div className="relative border-t border-white/10 px-8 py-6">
          <div className="mx-auto flex max-w-3xl flex-wrap items-center justify-center gap-x-8 gap-y-2 text-body-sm text-white/60">
            {ENGINE_STAGES.map((stage, i) => (
              <span key={stage} className="flex items-center gap-8">
                {stage}
                {i < ENGINE_STAGES.length - 1 && <span aria-hidden="true">→</span>}
              </span>
            ))}
          </div>
        </div>
      </div>

      <section className="mx-auto grid w-full max-w-5xl grid-cols-1 gap-6 px-8 py-20 sm:grid-cols-2">
        <Card>
          <CardTitle>One score, not seven reports</CardTitle>
          <CardDescription className="mt-2">
            Every engine's findings normalise into the same model, weighted into a single 0–100
            score with a per-engine breakdown you can drill into.
          </CardDescription>
        </Card>
        <Card>
          <CardTitle>Every finding, explained</CardTitle>
          <CardDescription className="mt-2">
            Plain-language impact first, exact location second, a deterministic fix third — the AI
            patch is an accelerator, never the only source of guidance.
          </CardDescription>
        </Card>
        <Card className="sm:col-span-2">
          <CardTitle>Sandboxed by default</CardTitle>
          <CardDescription className="mt-2">
            Pentest and container inspection run inside a no-network, read-only, non-root container
            with capabilities dropped — a security tool that isn't itself secure is worthless.
          </CardDescription>
        </Card>
      </section>

      <section
        className="relative overflow-hidden px-8 py-20 text-center text-text-inverse"
        style={{ background: 'linear-gradient(135deg, var(--glow-2), var(--glow-1))' }}
      >
        <h2 className="text-display-section" style={{ fontFamily: 'var(--font-display-serif)' }}>
          See your risk before it ships.
        </h2>
        <Link
          to="/register"
          className="mt-6 inline-block rounded-md bg-white px-6 py-3 text-body font-medium text-black hover:opacity-90"
        >
          Get started free
        </Link>
      </section>

      <PublicFooter />
    </div>
  )
}
