import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Check, Coins, Lock, Radio, ShieldCheck, Zap } from 'lucide-react'
import { PublicNav } from '../components/PublicNav'
import { PublicFooter } from '../components/PublicFooter'
import { Button } from '../components/ui/Button'
import { cn } from '../lib/cn'
import { ENGINE_META } from '../lib/engines'
import type { Engine } from '../lib/rulesApi'
import {
  billingErrorMessage,
  formatPrice,
  formatShortDate,
  formatTokens,
  formatTokensShort,
  getCatalog,
  startCheckout,
  type BillingCatalog,
  type BillingPlan,
} from '../lib/billingApi'
import { useAuthStore } from '../stores/authStore'
import { useBillingStore } from '../stores/billingStore'

const ENGINE_ORDER: Engine[] = [
  'depscan',
  'k8sscan',
  'cicdscan',
  'containerscan',
  'codescan',
  'docreview',
  'pentest',
]

/**
 * Public pricing page. Every number on it comes from GET /billing/catalog —
 * change a price in internal/modules/billing/catalog.go and this page
 * follows, with no frontend edit.
 */
export function PricingPage() {
  const [catalog, setCatalog] = useState<BillingCatalog | null>(null)
  const [annual, setAnnual] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()

  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const user = useAuthStore((s) => s.user)
  const summary = useBillingStore((s) => s.summary)
  const refresh = useBillingStore((s) => s.refresh)

  useEffect(() => {
    getCatalog()
      .then(setCatalog)
      .catch(() => setError('Could not load prices. Is the backend running?'))
  }, [])
  useEffect(() => {
    if (isAuthenticated) void refresh()
  }, [isAuthenticated, refresh])

  async function buy(code: string) {
    if (!isAuthenticated) {
      navigate('/register')
      return
    }
    setBusy(code)
    setError(null)
    try {
      const sess = await startCheckout(code)
      navigate(sess.redirect_url ?? `/checkout/${sess.id}`)
    } catch (err) {
      setError(billingErrorMessage(err) ?? 'Could not start checkout.')
      setBusy(null)
    }
  }

  const free = catalog?.plans.find((p) => p.code === 'free')
  const monthly = catalog?.plans.find((p) => p.code === 'pro_monthly')
  const yearly = catalog?.plans.find((p) => p.code === 'pro_annual')
  const pro = annual ? yearly : monthly
  const savings = monthly && yearly ? monthly.price_cents * 12 - yearly.price_cents : 0
  const savingsPct =
    monthly && yearly ? Math.round((savings / (monthly.price_cents * 12)) * 100) : 0
  const isAdmin = user?.role === 'admin'
  const periodActive = summary ? new Date(summary.period_end) > new Date() : false

  /** Why a plan's button is disabled, or null if it can be bought. */
  function planBlock(plan: BillingPlan): string | null {
    if (!isAuthenticated || !summary) return null
    if (summary.plan.code === plan.code && periodActive) {
      return `Current plan · ${summary.cancel_at_period_end ? 'ends' : 'renews'} ${formatShortDate(summary.period_end)}`
    }
    if (summary.plan.code === 'pro_annual' && plan.code === 'pro_monthly' && periodActive) {
      return `Included in your annual plan until ${formatShortDate(summary.period_end)}`
    }
    if (!isAdmin) return 'Ask an organization admin to upgrade'
    return null
  }

  return (
    <div className="flex min-h-screen flex-col">
      <div className="px-4 pt-6">
        <PublicNav />
      </div>

      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-16">
        <div className="text-center">
          <p className="text-caption font-semibold uppercase tracking-wide text-accent">Pricing</p>
          <h1
            className="mt-2 text-display-section"
            style={{ fontFamily: 'var(--font-display-serif)' }}
          >
            Pay for the scans you run.
          </h1>
          <p className="mx-auto mt-4 max-w-xl text-body text-text-secondary">
            Every plan comes with a monthly allowance of tokens. Each scan engine costs a fixed
            number of tokens, and an engine that doesn&rsquo;t apply to your project is refunded
            automatically.
          </p>

          <div
            role="radiogroup"
            aria-label="Billing period"
            className="mx-auto mt-8 inline-flex items-center rounded-full border border-border-default bg-bg-surface p-1 shadow-sm"
          >
            {[
              { v: false, label: 'Monthly' },
              { v: true, label: 'Annual' },
            ].map((o) => (
              <button
                key={o.label}
                type="button"
                role="radio"
                aria-checked={annual === o.v}
                onClick={() => setAnnual(o.v)}
                className={cn(
                  'rounded-full px-4 py-1.5 text-body-sm font-medium transition-colors',
                  annual === o.v
                    ? 'bg-accent text-text-inverse'
                    : 'text-text-secondary hover:text-text-primary',
                )}
              >
                {o.label}
                {o.v && savingsPct > 0 && (
                  <span
                    className={cn(
                      'ml-2 rounded-full px-1.5 py-0.5 text-caption font-semibold',
                      annual ? 'bg-white/20' : 'bg-success/15 text-success',
                    )}
                  >
                    Save {savingsPct}%
                  </span>
                )}
              </button>
            ))}
          </div>
        </div>

        {error && (
          <p
            role="alert"
            className="mx-auto mt-6 max-w-xl rounded-md border border-danger/30 bg-danger/5 px-4 py-2 text-center text-body-sm text-danger"
          >
            {error}
          </p>
        )}

        {!catalog ? (
          <p className="mt-12 text-center text-body-sm text-text-tertiary">Loading prices…</p>
        ) : (
          <>
            <div className="mt-10 grid grid-cols-1 gap-6 md:grid-cols-2">
              {free && (
                <PlanCard
                  plan={free}
                  price="$0"
                  priceNote="forever"
                  blurb="Try GuardPipe on a side project."
                  features={[
                    `${formatTokens(free.monthly_grant)} tokens every month`,
                    'Six engines: code, dependencies, containers, Kubernetes, CI/CD, design docs',
                    'Manual scans',
                  ]}
                  missing={['Penetration testing', 'Live scans on every push', 'Scheduled scans']}
                  cta={
                    isAuthenticated
                      ? null
                      : { label: 'Start free', onClick: () => navigate('/register') }
                  }
                  current={summary?.plan.code === 'free'}
                />
              )}
              {pro && monthly && (
                <PlanCard
                  highlight
                  plan={pro}
                  price={
                    annual
                      ? formatPrice(Math.round(pro.price_cents / 12))
                      : formatPrice(pro.price_cents)
                  }
                  priceNote={
                    annual
                      ? `per month, billed ${formatPrice(pro.price_cents)} yearly`
                      : 'per month'
                  }
                  blurb={`Every month: ${catalog.guarantee.pentests} pentests, ${catalog.guarantee.full_scans} full supply-chain scans, and ${catalog.guarantee.live_pushes}+ automatic scans on push.`}
                  features={[
                    `${formatTokens(pro.monthly_grant)} tokens every month${annual ? ' (granted monthly for 12 months)' : ''}`,
                    'All seven engines, including penetration testing',
                    `Live scans on every push at ${Math.round((1 - catalog.live_discount) * 100)}% off`,
                    'Scheduled scans',
                    ...(annual ? [`${pro.topup_discount_percent}% off top-up packs`] : []),
                  ]}
                  cta={{
                    label: `Get Pro — ${annual ? `${formatPrice(pro.price_cents)}/year` : `${formatPrice(pro.price_cents)}/month`}`,
                    onClick: () => void buy(pro.code),
                    loading: busy === pro.code,
                    disabledReason: planBlock(pro),
                  }}
                  current={summary?.plan.code === pro.code}
                  savings={
                    annual && savings > 0 ? `You save ${formatPrice(savings)} a year` : undefined
                  }
                />
              )}
            </div>

            <section className="mt-16">
              <h2 className="text-h2 text-text-primary">Need more tokens?</h2>
              <p className="mt-1 text-body-sm text-text-secondary">
                Top-up packs work on any plan, are used after your monthly tokens, and last 12
                months.
              </p>
              <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-3">
                {catalog.packs.map((pack) => {
                  const discount = summary?.plan.topup_discount_percent ?? 0
                  const price = Math.floor((pack.price_cents * (100 - discount)) / 100)
                  const blocked = isAuthenticated && !isAdmin
                  return (
                    <div
                      key={pack.code}
                      className="flex flex-col rounded-lg border border-border-default bg-bg-surface p-5 shadow-sm"
                    >
                      <div className="flex items-center gap-2 text-text-primary">
                        <Coins className="h-4 w-4 text-accent" aria-hidden="true" />
                        <span className="text-h3 font-semibold">
                          {formatTokensShort(pack.tokens)} tokens
                        </span>
                      </div>
                      <p className="mt-2 text-h2 font-semibold text-text-primary">
                        {formatPrice(price)}
                        {discount > 0 && (
                          <span className="ml-2 text-body-sm font-normal text-text-tertiary line-through">
                            {formatPrice(pack.price_cents)}
                          </span>
                        )}
                      </p>
                      <p className="text-caption text-text-tertiary">
                        {formatPrice(Math.round((price / pack.tokens) * 100_000))} per 100k
                      </p>
                      <Button
                        variant="secondary"
                        size="sm"
                        className="mt-4"
                        loading={busy === pack.code}
                        disabled={blocked}
                        title={blocked ? 'Ask an organization admin' : undefined}
                        onClick={() => void buy(pack.code)}
                      >
                        {isAuthenticated ? 'Buy' : 'Sign up to buy'}
                      </Button>
                    </div>
                  )
                })}
              </div>
            </section>

            <CostTable catalog={catalog} />
            <Faq catalog={catalog} />
          </>
        )}
      </main>

      <PublicFooter />
    </div>
  )
}

function PlanCard({
  plan,
  price,
  priceNote,
  blurb,
  features,
  missing = [],
  cta,
  current,
  highlight,
  savings,
}: {
  plan: BillingPlan
  price: string
  priceNote: string
  blurb: string
  features: string[]
  missing?: string[]
  cta: {
    label: string
    onClick: () => void
    loading?: boolean
    disabledReason?: string | null
  } | null
  current?: boolean
  highlight?: boolean
  savings?: string
}) {
  return (
    <div
      className={cn(
        'relative flex flex-col rounded-xl border bg-bg-surface p-7 shadow-sm',
        highlight ? 'border-accent shadow-md' : 'border-border-default',
      )}
    >
      {highlight && (
        <span className="absolute -top-3 left-7 rounded-full bg-accent px-3 py-0.5 text-caption font-semibold text-text-inverse">
          Most teams pick this
        </span>
      )}
      <div className="flex items-center justify-between">
        <h3 className="text-h2 text-text-primary">{plan.code === 'free' ? 'Free' : 'Pro'}</h3>
        {current && (
          <span className="rounded-full bg-success/15 px-2 py-0.5 text-caption font-semibold text-success">
            Your plan
          </span>
        )}
      </div>
      <p className="mt-4">
        <span className="text-display-section font-semibold text-text-primary">{price}</span>
        <span className="ml-2 text-body-sm text-text-secondary">{priceNote}</span>
      </p>
      {savings && <p className="mt-1 text-body-sm font-medium text-success">{savings}</p>}
      <p className="mt-3 text-body-sm text-text-secondary">{blurb}</p>

      <ul className="mt-5 flex-1 space-y-2">
        {features.map((f) => (
          <li key={f} className="flex gap-2 text-body-sm text-text-primary">
            <Check className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
            {f}
          </li>
        ))}
        {missing.map((f) => (
          <li key={f} className="flex gap-2 text-body-sm text-text-tertiary">
            <Lock className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            {f}
          </li>
        ))}
      </ul>

      {cta && (
        <div className="mt-6">
          <Button
            className="w-full"
            variant={highlight ? 'primary' : 'secondary'}
            loading={cta.loading}
            disabled={!!cta.disabledReason}
            onClick={cta.onClick}
          >
            {cta.disabledReason ?? cta.label}
          </Button>
        </div>
      )}
    </div>
  )
}

function CostTable({ catalog }: { catalog: BillingCatalog }) {
  const pentestBase = catalog.engine_prices.pentest ?? 0
  const liveOff = Math.round((1 - catalog.live_discount) * 100)
  return (
    <section className="mt-16 grid grid-cols-1 gap-8 lg:grid-cols-2">
      <div>
        <h2 className="text-h2 text-text-primary">What does a scan cost?</h2>
        <p className="mt-1 text-body-sm text-text-secondary">
          Each engine has a fixed price. A scan costs the sum of the engines it runs.
        </p>
        <table className="mt-4 w-full text-body-sm">
          <tbody>
            {ENGINE_ORDER.filter((e) => e !== 'pentest').map((e) => (
              <tr key={e} className="border-b border-border-default">
                <td className="py-2 text-text-primary">{ENGINE_META[e]?.label ?? e}</td>
                <td className="py-2 text-right tabular-nums text-text-primary">
                  {formatTokens(catalog.engine_prices[e] ?? 0)}
                </td>
              </tr>
            ))}
            {(['stealth', 'standard', 'deep'] as const).map((preset) => (
              <tr key={preset} className="border-b border-border-default">
                <td className="py-2 text-text-primary">
                  Pentest <span className="text-text-tertiary">· {preset}</span>
                </td>
                <td className="py-2 text-right tabular-nums text-text-primary">
                  {formatTokens(
                    Math.ceil((pentestBase * (catalog.pentest_multipliers[preset] ?? 1)) / 50) * 50,
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 flex items-center gap-2 text-body-sm text-text-secondary">
          <Radio className="h-4 w-4 text-accent" aria-hidden="true" />
          Live scans triggered by a GitHub push cost {liveOff}% less.
        </p>
      </div>

      <div>
        <h2 className="text-h2 text-text-primary">How far a Pro month goes</h2>
        <p className="mt-1 text-body-sm text-text-secondary">
          Each line on its own, out of {formatTokens(catalog.guarantee.monthly_allowance)} tokens.
        </p>
        <ul className="mt-4 space-y-2">
          {catalog.examples.map((ex) => (
            <li
              key={ex.key}
              className="flex items-center justify-between rounded-md border border-border-default bg-bg-surface px-4 py-2.5 text-body-sm"
            >
              <span className="text-text-primary">{ex.label}</span>
              <span className="text-right tabular-nums">
                <span className="font-semibold text-text-primary">~{ex.per_pro_month}×</span>
                <span className="ml-2 text-text-tertiary">{formatTokens(ex.tokens)} each</span>
              </span>
            </li>
          ))}
        </ul>
        <div className="mt-4 flex gap-3 rounded-lg border border-accent/30 bg-accent/5 p-4 text-body-sm text-text-primary">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span>
            Or mix them: {catalog.guarantee.pentests} standard pentests +{' '}
            {catalog.guarantee.full_scans} full scans + {catalog.guarantee.live_pushes} live pushes
            = {formatTokens(catalog.guarantee.tokens_needed)} tokens, inside one Pro month.
          </span>
        </div>
      </div>
    </section>
  )
}

function Faq({ catalog }: { catalog: BillingCatalog }) {
  const items = [
    {
      q: 'Do unused tokens roll over?',
      a: 'Monthly plan tokens reset at the start of each month. Tokens from top-up packs carry over and last 12 months, and they are only used after your monthly tokens run out.',
    },
    {
      q: 'What if an engine doesn’t apply to my project?',
      a: 'You get those tokens back automatically. A project with no Dockerfile gets the container scan refunded, and an engine that fails on our side is refunded too.',
    },
    {
      q: 'How do live scans work?',
      a: `Turn on live scanning for a project and every push to a watched branch starts a scan at ${Math.round((1 - catalog.live_discount) * 100)}% off. The same commit is never scanned twice, and live scans pause by themselves before they use up the tokens you need for manual scans.`,
    },
    {
      q: 'What happens when I run out?',
      a: 'Nothing breaks: you just can’t start a new scan until the month resets or you buy a top-up. You always see the cost before a scan starts.',
    },
    {
      q: 'Is this a real payment?',
      a: 'Not yet — checkout is in demo mode. No card details are collected or sent anywhere.',
    },
  ]
  return (
    <section className="mt-16">
      <h2 className="text-h2 text-text-primary">Questions</h2>
      <dl className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        {items.map((i) => (
          <div key={i.q} className="rounded-lg border border-border-default bg-bg-surface p-5">
            <dt className="flex items-center gap-2 font-semibold text-text-primary">
              <Zap className="h-4 w-4 text-accent" aria-hidden="true" />
              {i.q}
            </dt>
            <dd className="mt-2 text-body-sm text-text-secondary">{i.a}</dd>
          </div>
        ))}
      </dl>
      <p className="mt-8 text-center text-body-sm text-text-secondary">
        Already have an account?{' '}
        <Link to="/settings/billing" className="font-medium text-accent hover:underline">
          See your usage
        </Link>
      </p>
    </section>
  )
}
