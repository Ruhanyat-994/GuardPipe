import { Link } from 'react-router-dom'
import {
  FileText,
  Code2,
  Package,
  Container as ContainerIcon,
  Boxes,
  Workflow,
  ShieldAlert,
} from 'lucide-react'
import { HeroGlow } from '../components/HeroGlow'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'

const ENGINE_STAGES = [
  { label: 'Docs', icon: FileText },
  { label: 'Code', icon: Code2 },
  { label: 'Deps', icon: Package },
  { label: 'Containers', icon: ContainerIcon },
  { label: 'K8s', icon: Boxes },
  { label: 'CI/CD', icon: Workflow },
  { label: 'Pentest', icon: ShieldAlert },
]

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
      <div className="relative overflow-hidden text-public-fg">
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

        <div className="relative border-t border-white/10 px-8 py-10">
          <div className="mx-auto flex max-w-4xl items-start justify-center overflow-x-auto">
            {ENGINE_STAGES.map((stage, i) => (
              <div key={stage.label} className="flex items-start">
                <div className="flex w-16 flex-col items-center gap-2 sm:w-20">
                  <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full border border-white/15 bg-white/5">
                    <stage.icon className="h-5 w-5 text-white/80" aria-hidden="true" />
                  </div>
                  <span className="text-caption whitespace-nowrap text-white/60">
                    {stage.label}
                  </span>
                </div>
                {i < ENGINE_STAGES.length - 1 && (
                  <div
                    className="mt-[22px] h-px w-4 shrink-0 bg-white/15 sm:w-8"
                    aria-hidden="true"
                  />
                )}
              </div>
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
        className="relative overflow-hidden px-8 py-20 text-center text-public-fg"
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
