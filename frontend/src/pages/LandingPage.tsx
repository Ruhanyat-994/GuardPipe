import { type ReactNode, useEffect, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import {
  Boxes,
  ChevronRight,
  Code2,
  Container as ContainerIcon,
  FileText,
  Gauge,
  Package,
  Plus,
  ShieldAlert,
  Workflow,
} from 'lucide-react'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { AuroraBars } from '../components/landing/AuroraBars'
import { SecurePipeFlow } from '../components/landing/SecurePipeFlow'
import { useCountUp } from '../hooks/useCountUp'
import { cn } from '../lib/cn'
import { useAuthStore } from '../stores/authStore'

/**
 * The public landing page. Structure and motion language adapted from a
 * reference site the product owner supplied (dark long-exposure hero with a
 * single floating 3D object, nav that collapses into a pill, dashed column
 * guides, alternating dark "space" bands and light sections, a giant
 * outlined wordmark in the footer) — rebuilt in GuardPipe's own blue
 * palette. The hero's motion graphic (SecurePipeFlow) follows a second
 * reference: messy input flowing through a middleman and leaving clean —
 * here, vulnerabilities flowing along git-branch-shaped pipes into
 * GuardPipe and out as one secured stream.
 *
 * Every number and claim on this page is a real product fact — no invented
 * customer logos or vanity metrics.
 */

const ENGINES = [
  {
    icon: FileText,
    name: 'Design docs',
    blurb: 'Threat-model gaps and prompt-injection attempts in your specs.',
  },
  {
    icon: Code2,
    name: 'Code',
    blurb: 'Security-relevant static analysis, filtered to what matters.',
  },
  {
    icon: Package,
    name: 'Dependencies',
    blurb: 'Known-vulnerable packages and secrets committed by mistake.',
  },
  {
    icon: ContainerIcon,
    name: 'Containers',
    blurb: 'Dockerfile misconfigurations and vulnerable image layers.',
  },
  {
    icon: Boxes,
    name: 'Kubernetes',
    blurb: 'Manifests and Helm charts, rendered offline and policy-checked.',
  },
  { icon: Workflow, name: 'CI/CD', blurb: 'Pipelines that leak secrets or run untrusted code.' },
  {
    icon: ShieldAlert,
    name: 'Pentest',
    blurb: 'A sandboxed live probe of a target you own — never automatic.',
  },
]

const ECOSYSTEMS = [
  'npm',
  'PyPI',
  'Go modules',
  'Maven',
  'Composer',
  'Dockerfiles',
  'Container images',
  'Kubernetes',
  'Helm',
  'GitHub Actions',
  'OpenAPI',
  'Markdown specs',
]

const FAQS = [
  {
    q: 'Does GuardPipe run my code?',
    a: 'No. Code, dependency, container, Kubernetes and pipeline analysis only read files. The one thing that executes anything is the pentest, and it runs inside a locked-down sandbox — no network except to your attested target, read-only, non-root, every capability dropped, time-limited.',
  },
  {
    q: 'What does the 0–100 score mean?',
    a: 'It is one explainable number built from every finding every engine produced, weighted by severity and confidence, with a per-engine breakdown you can open. It maps to a verdict — pass, warn or block — you can use as a gate.',
  },
  {
    q: 'What happens to my GitHub token?',
    a: 'It is encrypted with AES-256-GCM the moment it reaches the server and is never returned by any API response. You only ever see a masked hint.',
  },
  {
    q: 'What does a quick test cost?',
    a: 'Nothing. The Free plan includes 15,000 tokens every month — enough for several partial scans. Engines that don’t apply to your repo, or that fail on our side, are refunded automatically.',
  },
  {
    q: 'Do I have to watch the scan?',
    a: 'No. Scans run on the server. Leave the page, keep working — a running-scans indicator follows you, and the PDF report can land in your inbox when it’s done.',
  },
]

function useInView<T extends Element>(threshold = 0.25) {
  const ref = useRef<T>(null)
  const [inView, setInView] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const obs = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setInView(true)
          obs.disconnect()
        }
      },
      { threshold },
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [threshold])
  return { ref, inView }
}

/** Fades and lifts its children in the first time they scroll into view. */
function Reveal({
  children,
  className,
  delay = 0,
}: {
  children: ReactNode
  className?: string
  delay?: number
}) {
  const { ref, inView } = useInView<HTMLDivElement>(0.15)
  return (
    <div
      ref={ref}
      className={cn('gp-reveal', inView && 'is-visible', className)}
      style={{ transitionDelay: `${delay}ms` }}
    >
      {children}
    </div>
  )
}

function Stat({
  value,
  suffix,
  prefix,
  label,
  run,
}: {
  value: number
  suffix?: string
  prefix?: string
  label: string
  run: boolean
}) {
  const n = useCountUp(run ? value : 0, 1400)
  return (
    <div className="flex flex-col items-center gap-2 px-6 text-center">
      <p className="text-[44px] leading-none font-semibold tracking-tight text-white tabular-nums">
        {prefix}
        {n}
        {suffix}
      </p>
      <p className="max-w-[200px] text-[13px] leading-5 text-white/55">{label}</p>
    </div>
  )
}

export function LandingPage() {
  const { hash } = useLocation()

  // `/#engines` (nav, footer) — React Router doesn't scroll to hashes itself.
  useEffect(() => {
    if (!hash) return
    document.getElementById(hash.slice(1))?.scrollIntoView({ behavior: 'smooth' })
  }, [hash])
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const ctaTo = isAuthenticated ? '/projects' : '/register'
  const { ref: numbersRef, inView: numbersInView } = useInView<HTMLDivElement>(0.4)

  return (
    <div className="flex min-h-screen flex-col bg-[#f7f7f5] text-neutral-900">
      <PublicNav variant="hero" />

      {/* ─── Hero ─────────────────────────────────────────────── */}
      <section className="relative isolate overflow-hidden bg-[#020106] text-white">
        <AuroraBars />
        <div
          aria-hidden="true"
          className="absolute inset-x-0 bottom-0 h-40 bg-gradient-to-b from-transparent to-[#020106]"
        />

        {/* The flow first: vulnerabilities in (red), GuardPipe in the middle,
            fixed versions out (blue). Full-bleed, like the pipes run past
            the page edges. */}
        <div
          className="gp-hero-in relative mt-24 h-[300px] w-full sm:h-[360px] lg:h-[26.5vw] lg:max-h-[540px]"
          style={{ animationDelay: '80ms' }}
        >
          {/* soft dark backdrop so the thin strings read over the stars */}
          <div
            aria-hidden="true"
            className="absolute inset-0"
            style={{
              background:
                'radial-gradient(70% 62% at 50% 55%, rgba(2,1,6,0.85) 0%, rgba(2,1,6,0.5) 55%, transparent 82%)',
            }}
          />
          <div className="relative h-full w-full">
            <SecurePipeFlow />
          </div>
        </div>

        {/* …then the headline, underneath the pipe */}
        <div className="relative mx-auto max-w-5xl border-x border-dashed border-white/[0.08] px-6 pt-10 text-center">
          <div className="gp-hero-in" style={{ animationDelay: '200ms' }}>
            <h1 className="text-[40px] leading-[1.08] font-semibold tracking-tight sm:text-[56px]">
              Secure every layer of what you ship.
            </h1>
            <div className="mt-8 flex justify-center">
              <Link
                to={ctaTo}
                className="group inline-flex items-center gap-1.5 rounded-full bg-white px-6 py-3 text-[15px] font-semibold text-neutral-950 shadow-[0_0_40px_rgba(143,227,255,0.25)] transition-shadow hover:shadow-[0_0_60px_rgba(143,227,255,0.55)]"
              >
                {isAuthenticated ? 'Open dashboard' : 'Run a quick test'}
                <ChevronRight
                  className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                  aria-hidden="true"
                />
              </Link>
            </div>
          </div>
        </div>

        {/* ecosystem marquee */}
        <div className="relative mx-auto max-w-5xl border-x border-dashed border-white/[0.08] pt-10 pb-14">
          <p className="mb-5 text-center text-[12px] tracking-wide text-white/40">
            Reads what you already ship
          </p>
          <div className="gp-marquee-mask overflow-hidden">
            <div className="gp-marquee flex w-max gap-12">
              {[...ECOSYSTEMS, ...ECOSYSTEMS].map((e, i) => (
                <span key={i} className="text-[17px] font-semibold whitespace-nowrap text-white/45">
                  {e}
                </span>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* ─── Engines grid (light) ─────────────────────────────── */}
      <section id="engines" className="scroll-mt-24">
        <div className="mx-auto max-w-5xl border-x border-dashed border-black/[0.08]">
          <Reveal className="px-6 pt-24 pb-12 text-center">
            <p className="text-[13px] text-neutral-500">Seven engines, one pipeline</p>
            <h2 className="mx-auto mt-3 max-w-xl text-[34px] leading-tight font-semibold tracking-tight">
              Every layer of the supply chain, in a single scan.
            </h2>
          </Reveal>
          <div className="grid grid-cols-1 border-t border-dashed border-black/[0.08] sm:grid-cols-2 lg:grid-cols-4">
            {ENGINES.map((e, i) => (
              <Reveal
                key={e.name}
                delay={i * 60}
                className="group border-b border-dashed border-black/[0.08] p-7 transition-colors hover:bg-white sm:border-r"
              >
                <e.icon
                  className="h-5 w-5 text-neutral-800 transition-colors group-hover:text-[#2563eb]"
                  aria-hidden="true"
                />
                <p className="mt-6 text-[15px] font-semibold">{e.name}</p>
                <p className="mt-1.5 text-[13px] leading-5 text-neutral-500">{e.blurb}</p>
              </Reveal>
            ))}
            <Reveal
              delay={ENGINES.length * 60}
              className="border-b border-dashed border-black/[0.08] bg-neutral-950 p-7 text-white"
            >
              <Gauge className="h-5 w-5 text-[#8fe3ff]" aria-hidden="true" />
              <p className="mt-6 text-[15px] font-semibold">One verdict</p>
              <p className="mt-1.5 text-[13px] leading-5 text-white/60">
                All seven normalised into one finding model and one 0–100 score: pass, warn or
                block.
              </p>
            </Reveal>
          </div>
          <div className="h-24" />
        </div>
      </section>

      {/* ─── By the numbers (dark, planet horizon) ────────────── */}
      <section className="relative overflow-hidden bg-[#03050b]">
        <div className="relative mx-auto max-w-5xl border-x border-dashed border-white/[0.08] px-6 pt-28 pb-64">
          <Reveal>
            <h2 className="text-center text-[34px] leading-tight font-semibold tracking-tight text-white">
              One scan,
              <br />
              by the numbers.
            </h2>
          </Reveal>
          <div
            ref={numbersRef}
            className="mt-20 grid grid-cols-1 gap-12 sm:grid-cols-3 sm:divide-x sm:divide-white/10"
          >
            <Stat
              run={numbersInView}
              value={7}
              label="supply-chain layers checked in a single run"
            />
            <Stat
              run={numbersInView}
              value={100}
              prefix="0–"
              label="one explainable risk score, with its per-engine breakdown"
            />
            <Stat
              run={numbersInView}
              value={50}
              suffix="%"
              label="off every live scan triggered by a push to GitHub"
            />
          </div>
        </div>
        {/* planet horizon */}
        <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-0 h-72">
          <div
            className="absolute left-1/2 top-24 h-[1400px] w-[2200px] -translate-x-1/2 rounded-[50%]"
            style={{
              background: 'radial-gradient(closest-side, #0e1a44 0%, #081028 55%, #03050b 100%)',
              boxShadow:
                '0 -18px 60px rgba(111,147,255,0.35), inset 0 12px 40px rgba(143,227,255,0.25)',
            }}
          />
        </div>
      </section>

      {/* ─── Three capability cards (dark) ────────────────────── */}
      <section className="bg-[#03050b] pb-28">
        <div className="mx-auto grid max-w-5xl grid-cols-1 gap-4 border-x border-dashed border-white/[0.08] px-6 md:grid-cols-3">
          {[
            {
              title: 'Live scanning',
              body: 'Every push to a watched branch starts a scan with the engines you chose. Never a pentest.',
              art: <ArtLive />,
            },
            {
              title: 'Sandboxed pentest',
              body: 'Probe a target you’ve attested ownership of, from a no-network-by-default sandbox.',
              art: <ArtSandbox />,
            },
            {
              title: 'Explained, not dumped',
              body: 'Plain-language impact, the exact location, a deterministic fix — and an AI patch on top.',
              art: <ArtExplain />,
            },
          ].map((c, i) => (
            <Reveal key={c.title} delay={i * 90}>
              <div className="group h-full overflow-hidden rounded-2xl border border-white/10 bg-gradient-to-b from-[#0b1330] to-[#05070c] transition-transform duration-300 hover:-translate-y-1">
                <div className="h-48 border-b border-white/5">{c.art}</div>
                <div className="p-6">
                  <p className="text-[15px] font-semibold text-white">{c.title}</p>
                  <p className="mt-2 text-[13px] leading-5 text-white/55">{c.body}</p>
                </div>
              </div>
            </Reveal>
          ))}
        </div>
      </section>

      {/* ─── How a scan flows (light split) ───────────────────── */}
      <section>
        <div className="mx-auto grid max-w-5xl grid-cols-1 items-center gap-12 border-x border-dashed border-black/[0.08] px-6 py-28 lg:grid-cols-2">
          <Reveal>
            <p className="text-[13px] text-neutral-500">Fire and forget</p>
            <h2 className="mt-2 text-[34px] leading-tight font-semibold tracking-tight">
              Click once. Get on with your day.
            </h2>
            <p className="mt-5 max-w-md text-[15px] leading-6 text-neutral-600">
              A scan runs on the server while you use the rest of GuardPipe. When it finishes you
              get a notification — and, if you want it, the PDF report in your inbox.
            </p>
            <Link
              to={ctaTo}
              className="group mt-8 inline-flex items-center gap-1.5 rounded-full bg-neutral-950 px-5 py-2.5 text-[14px] font-semibold text-white hover:bg-neutral-800"
            >
              Run a quick test
              <ChevronRight
                className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                aria-hidden="true"
              />
            </Link>
          </Reveal>
          <Reveal delay={120}>
            <ScanTimeline />
          </Reveal>
        </div>
      </section>

      {/* ─── FAQ ──────────────────────────────────────────────── */}
      <section>
        <div className="mx-auto max-w-5xl border-x border-t border-dashed border-black/[0.08] px-6 py-28">
          <h2 className="text-center text-[34px] font-semibold tracking-tight">FAQs</h2>
          <div className="mx-auto mt-12 flex max-w-2xl flex-col gap-3">
            {FAQS.map((f) => (
              <details
                key={f.q}
                className="group rounded-xl bg-white px-5 py-4 shadow-[0_1px_0_rgba(0,0,0,0.04)] ring-1 ring-black/5"
              >
                <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-[14px] font-medium">
                  {f.q}
                  <Plus
                    className="h-4 w-4 shrink-0 text-neutral-400 transition-transform group-open:rotate-45"
                    aria-hidden="true"
                  />
                </summary>
                <p className="mt-3 text-[14px] leading-6 text-neutral-600">{f.a}</p>
              </details>
            ))}
          </div>
        </div>
      </section>

      {/* ─── Closing CTA ──────────────────────────────────────── */}
      <section className="relative overflow-hidden bg-[#03050b] text-white">
        {/* a planet rising from the top edge, its ring crossing in front */}
        <svg
          aria-hidden="true"
          viewBox="0 0 1200 260"
          preserveAspectRatio="xMidYMin slice"
          className="pointer-events-none absolute inset-x-0 top-0 h-[260px] w-full"
        >
          <defs>
            <radialGradient id="cta-planet" cx="42%" cy="95%" r="70%">
              <stop offset="0%" stopColor="#23398f" />
              <stop offset="45%" stopColor="#0c1540" />
              <stop offset="100%" stopColor="#03050b" />
            </radialGradient>
            <linearGradient id="cta-ring" x1="0" x2="1">
              <stop offset="0%" stopColor="#6f93ff" stopOpacity="0" />
              <stop offset="50%" stopColor="#c9d8ff" stopOpacity="0.8" />
              <stop offset="100%" stopColor="#6f93ff" stopOpacity="0" />
            </linearGradient>
          </defs>
          <circle cx="600" cy="-190" r="330" fill="url(#cta-planet)" />
          <circle
            cx="600"
            cy="-190"
            r="330"
            fill="none"
            stroke="#8fe3ff"
            strokeOpacity="0.5"
            strokeWidth="1.2"
            style={{ filter: 'drop-shadow(0 6px 18px rgba(111,147,255,0.55))' }}
          />
          <ellipse
            cx="600"
            cy="70"
            rx="560"
            ry="46"
            fill="none"
            stroke="url(#cta-ring)"
            strokeWidth="2"
            transform="rotate(-4 600 70)"
          />
          <ellipse
            cx="600"
            cy="70"
            rx="600"
            ry="54"
            fill="none"
            stroke="url(#cta-ring)"
            strokeWidth="8"
            strokeOpacity="0.15"
            transform="rotate(-4 600 70)"
          />
        </svg>
        <div className="relative mx-auto max-w-5xl border-x border-dashed border-white/[0.08] px-6 pt-60 pb-28 text-center">
          <h2 className="text-[36px] font-semibold tracking-tight">Secure every layer.</h2>
          <p className="mx-auto mt-3 max-w-sm text-[15px] text-white/65">
            Point GuardPipe at a repository and get one honest verdict in minutes. Free to start.
          </p>
          <div className="mt-8 flex items-center justify-center gap-5">
            <Link
              to={ctaTo}
              className="group inline-flex items-center gap-1.5 rounded-full bg-white px-5 py-2.5 text-[14px] font-semibold text-neutral-950"
            >
              Run a quick test
              <ChevronRight
                className="h-4 w-4 transition-transform group-hover:translate-x-0.5"
                aria-hidden="true"
              />
            </Link>
            {!isAuthenticated && (
              <Link
                to="/login"
                className="text-[14px] font-semibold text-white/80 hover:text-white"
              >
                Sign in
              </Link>
            )}
          </div>
        </div>
      </section>

      <PublicFooter />
    </div>
  )
}

/** Animated "what happens after you click" card for the split section. */
function ScanTimeline() {
  const steps = [
    { t: '00:00', text: 'Scan queued — 7 engines, tokens reserved' },
    { t: '00:02', text: 'Repository cloned (shallow, size-checked)' },
    { t: '00:41', text: 'Dependencies · 2 high, 1 secret' },
    { t: '01:12', text: 'Kubernetes · Helm chart rendered offline' },
    { t: '02:05', text: 'Containerscan skipped — no Dockerfile · refunded' },
    { t: '02:30', text: 'Risk 38 / 100 · Warn — report emailed' },
  ]
  return (
    <div className="rounded-2xl bg-neutral-950 p-6 text-white shadow-[0_30px_80px_rgba(3,5,11,0.35)] ring-1 ring-white/10">
      <div className="mb-5 flex items-center gap-1.5">
        <span className="h-2.5 w-2.5 rounded-full bg-white/15" />
        <span className="h-2.5 w-2.5 rounded-full bg-white/15" />
        <span className="h-2.5 w-2.5 rounded-full bg-white/15" />
        <span className="ml-3 text-[12px] text-white/40">scan #42 · payments-api</span>
      </div>
      <ol className="flex flex-col gap-3 font-mono text-[12.5px]">
        {steps.map((s, i) => (
          <li key={s.t} className="gp-step flex gap-4" style={{ animationDelay: `${i * 0.55}s` }}>
            <span className="text-white/35">{s.t}</span>
            <span className={i === steps.length - 1 ? 'text-[#8fe3ff]' : 'text-white/80'}>
              {s.text}
            </span>
          </li>
        ))}
      </ol>
    </div>
  )
}

function ArtLive() {
  return (
    <svg viewBox="0 0 300 180" className="h-full w-full" aria-hidden="true">
      <path d="M20 120 H280" stroke="rgba(157,184,255,0.35)" strokeWidth="1.5" />
      <path
        d="M80 120 C110 120 110 70 140 70 H230"
        stroke="rgba(157,184,255,0.25)"
        strokeWidth="1.5"
        fill="none"
      />
      {[50, 120, 190, 250].map((x) => (
        <circle key={x} cx={x} cy={120} r="5" fill="#0b1330" stroke="#9db8ff" />
      ))}
      {[160, 215].map((x) => (
        <circle key={x} cx={x} cy={70} r="5" fill="#0b1330" stroke="#9db8ff" />
      ))}
      <circle r="4" fill="#8fe3ff" className="gp-commit-pulse">
        <animateMotion dur="3.6s" repeatCount="indefinite" path="M20 120 H280" />
      </circle>
      <text
        x="20"
        y="40"
        fill="rgba(255,255,255,0.45)"
        fontSize="11"
        fontFamily="ui-monospace, monospace"
      >
        git push origin main
      </text>
      <text x="20" y="152" fill="#8fe3ff" fontSize="11" fontFamily="ui-monospace, monospace">
        → scan started · 4 engines
      </text>
    </svg>
  )
}

function ArtSandbox() {
  return (
    <svg viewBox="0 0 300 180" className="h-full w-full" aria-hidden="true">
      <rect
        x="95"
        y="40"
        width="110"
        height="100"
        rx="14"
        fill="none"
        stroke="rgba(157,184,255,0.55)"
        strokeDasharray="5 5"
      />
      <rect x="125" y="68" width="50" height="44" rx="8" fill="#0b1330" stroke="#9db8ff" />
      <path d="M150 80 v10 M150 96 v2" stroke="#8fe3ff" strokeWidth="2.5" strokeLinecap="round" />
      <g className="gp-radar" style={{ transformOrigin: '150px 90px' }}>
        <path d="M150 90 L150 20 A70 70 0 0 1 206 48 Z" fill="url(#radarGrad)" />
      </g>
      <defs>
        <linearGradient id="radarGrad" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#8fe3ff" stopOpacity="0" />
          <stop offset="100%" stopColor="#8fe3ff" stopOpacity="0.25" />
        </linearGradient>
      </defs>
      <circle cx="252" cy="54" r="5" fill="#34d399" />
      <text
        x="232"
        y="80"
        fill="rgba(255,255,255,0.45)"
        fontSize="10"
        fontFamily="ui-monospace, monospace"
      >
        target
      </text>
      <path d="M205 80 L246 58" stroke="rgba(52,211,153,0.6)" strokeDasharray="3 4" />
    </svg>
  )
}

function ArtExplain() {
  return (
    <svg viewBox="0 0 300 180" className="h-full w-full" aria-hidden="true">
      <rect
        x="24"
        y="30"
        width="252"
        height="34"
        rx="8"
        fill="rgba(239,68,68,0.08)"
        stroke="rgba(239,68,68,0.35)"
      />
      <text x="38" y="52" fill="#fca5a5" fontSize="11" fontFamily="ui-monospace, monospace">
        codescan.injection.sql-concat
      </text>
      <path d="M150 70 v14" stroke="rgba(157,184,255,0.5)" />
      {[96, 114, 132].map((y, i) => (
        <rect
          key={y}
          x="24"
          y={y}
          width={[236, 200, 150][i]}
          height="8"
          rx="4"
          fill="rgba(157,184,255,0.25)"
          className="gp-typing"
          style={{ animationDelay: `${i * 0.4}s` }}
        />
      ))}
      <text x="24" y="165" fill="#8fe3ff" fontSize="11" fontFamily="ui-monospace, monospace">
        + use a parameterised query
      </text>
    </svg>
  )
}
