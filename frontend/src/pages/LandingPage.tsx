import { Link } from 'react-router-dom'
import { Button } from '../components/ui/Button'
import { Input } from '../components/ui/Input'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'

/**
 * Phase 1 stand-in for the real Landing page (documentation/09-ui-ux-design-system.md
 * §5.9) — the actual dark-hero, animated-glow design comes with its own
 * phase once the frontend design brief is in. For now this route doubles as
 * the "base component library on a static shell page" deliverable
 * BUILD_GUIDE.md's Phase 1 asks for: every primitive, every token, exercised
 * on one page with no live data.
 */
export function LandingPage() {
  return (
    <main className="mx-auto max-w-3xl px-8 py-16">
      <h1 className="text-display text-text-primary">GuardPipe</h1>
      <p className="mt-2 text-body text-text-secondary">
        Design tokens, base components, and routing — Phase 1 shared kernel, frontend half.
      </p>

      <nav className="mt-6 flex flex-wrap gap-3 text-body-sm">
        <Link className="text-accent underline" to="/login">
          Login
        </Link>
        <Link className="text-accent underline" to="/register">
          Register
        </Link>
        <Link className="text-accent underline" to="/blog">
          Blog
        </Link>
        <Link className="text-accent underline" to="/guides">
          Guides
        </Link>
        <Link className="text-accent underline" to="/projects">
          Projects (protected)
        </Link>
      </nav>

      <section className="mt-10 flex flex-col gap-6">
        <Card>
          <CardTitle>Buttons</CardTitle>
          <CardDescription className="mt-1 mb-4">
            Variants × sizes, per documentation/09-ui-ux-design-system.md §4.1.
          </CardDescription>
          <div className="flex flex-wrap items-center gap-3">
            <Button variant="primary">Primary</Button>
            <Button variant="secondary">Secondary</Button>
            <Button variant="ghost">Ghost</Button>
            <Button variant="destructive">Destructive</Button>
            <Button variant="primary" loading>
              Loading
            </Button>
            <Button variant="primary" disabled>
              Disabled
            </Button>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-3">
            <Button size="sm">Small</Button>
            <Button size="md">Medium</Button>
            <Button size="lg">Large</Button>
          </div>
        </Card>

        <Card>
          <CardTitle>Inputs</CardTitle>
          <CardDescription className="mt-1 mb-4">Default and invalid states.</CardDescription>
          <div className="flex flex-col gap-3">
            <Input placeholder="you@example.com" />
            <Input placeholder="Invalid state" invalid defaultValue="not-an-email" />
            <Input placeholder="Disabled" disabled />
          </div>
        </Card>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-5">
          {(['critical', 'high', 'medium', 'low', 'info'] as const).map((sev) => (
            <div
              key={sev}
              className="rounded-md border p-3 text-center text-body-sm font-medium"
              style={{
                backgroundColor: `var(--sev-${sev}-bg)`,
                borderColor: `var(--sev-${sev}-border)`,
                color: `var(--sev-${sev})`,
              }}
            >
              {sev}
            </div>
          ))}
        </div>
      </section>
    </main>
  )
}
