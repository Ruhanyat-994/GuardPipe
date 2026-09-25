import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, CheckCircle2, Coins, CreditCard, FlaskConical, Lock } from 'lucide-react'
import { Logo } from '../components/Logo'
import { Button } from '../components/ui/Button'
import { Input } from '../components/ui/Input'
import {
  billingErrorMessage,
  confirmCheckout,
  formatPrice,
  formatTokens,
  getCheckout,
  type CheckoutSession,
} from '../lib/billingApi'
import { notifyBillingChanged } from '../lib/billingEvents'

/**
 * Demo checkout (GUARDPIPE_BILLING_MODE=demo). It looks like a payment page
 * so the purchase flow can be shown end to end, but it isn't one: the card
 * fields are uncontrolled, never read, and never sent — the only thing the
 * backend ever receives is "success" or "declined". A real provider
 * (Stripe) replaces this page later behind the same backend seam.
 */
export function CheckoutPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [sess, setSess] = useState<CheckoutSession | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [paying, setPaying] = useState<'success' | 'declined' | null>(null)

  useEffect(() => {
    getCheckout(id)
      .then(setSess)
      .catch((err) => setLoadError(billingErrorMessage(err) ?? 'This checkout could not be found.'))
  }, [id])

  async function pay(outcome: 'success' | 'declined') {
    setPaying(outcome)
    setError(null)
    try {
      const done = await confirmCheckout(id, outcome)
      setSess(done)
      notifyBillingChanged()
      navigate(`/settings/billing?purchased=${encodeURIComponent(done.item_code)}`)
    } catch (err) {
      setError(billingErrorMessage(err) ?? 'The payment could not be completed.')
      setPaying(null)
      getCheckout(id)
        .then(setSess)
        .catch(() => {})
    }
  }

  const closed = sess && sess.status !== 'pending'

  return (
    <div className="min-h-screen bg-bg-base">
      <div className="border-b border-warning/40 bg-warning/10 px-4 py-2 text-center text-body-sm text-text-primary">
        <FlaskConical className="mr-1.5 inline h-4 w-4 text-warning" aria-hidden="true" />
        <strong>Demo mode</strong> — no real payment is taken and no card details leave this page.
      </div>

      <div className="mx-auto max-w-4xl px-6 py-10">
        <div className="mb-8 flex items-center justify-between">
          <Link
            to="/pricing"
            className="flex items-center gap-1.5 text-body-sm text-text-secondary hover:text-text-primary"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            Back to pricing
          </Link>
          <span className="flex items-center gap-2 text-h3 font-semibold text-text-primary">
            <Logo className="h-5 w-auto" /> GuardPipe
          </span>
        </div>

        {loadError ? (
          <div className="rounded-lg border border-border-default bg-bg-surface p-8 text-center">
            <p className="text-body text-text-primary">{loadError}</p>
            <Link
              to="/pricing"
              className="mt-4 inline-block text-body-sm font-medium text-accent hover:underline"
            >
              Start again from the pricing page
            </Link>
          </div>
        ) : !sess ? (
          <p className="text-body-sm text-text-tertiary">Loading checkout…</p>
        ) : (
          <div className="grid grid-cols-1 gap-8 md:grid-cols-5">
            <section className="rounded-xl border border-border-default bg-bg-surface p-6 shadow-sm md:col-span-2">
              <p className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
                Order summary
              </p>
              <h1 className="mt-2 text-h2 text-text-primary">{sess.item_name}</h1>
              <p className="mt-1 text-body-sm text-text-secondary">
                {sess.item_kind === 'plan'
                  ? sess.item_code === 'pro_annual'
                    ? 'Billed yearly. Tokens are granted every month.'
                    : 'Billed monthly.'
                  : 'One-time top-up. Lasts 12 months.'}
              </p>
              <div className="mt-5 flex items-center gap-2 rounded-md bg-accent/5 px-3 py-2 text-body-sm text-text-primary">
                <Coins className="h-4 w-4 text-accent" aria-hidden="true" />
                <span>
                  <strong>{formatTokens(sess.tokens)}</strong> tokens
                  {sess.item_kind === 'plan' ? ' every month' : ''}
                </span>
              </div>
              <div className="mt-6 flex items-baseline justify-between border-t border-border-default pt-4">
                <span className="text-body text-text-secondary">Total today</span>
                <span className="text-h2 font-semibold text-text-primary">
                  {formatPrice(sess.amount_cents)}
                </span>
              </div>
            </section>

            <section className="rounded-xl border border-border-default bg-bg-surface p-6 shadow-sm md:col-span-3">
              {sess.status === 'paid' ? (
                <div className="py-8 text-center">
                  <CheckCircle2 className="mx-auto h-10 w-10 text-success" aria-hidden="true" />
                  <p className="mt-3 text-h3 text-text-primary">Payment complete</p>
                  <Link
                    to="/settings/billing"
                    className="mt-4 inline-block text-body-sm font-medium text-accent hover:underline"
                  >
                    Go to billing
                  </Link>
                </div>
              ) : closed ? (
                <div className="py-8 text-center">
                  <p className="text-body text-text-primary">
                    {sess.status === 'expired'
                      ? 'This checkout expired.'
                      : 'This checkout was closed after a declined payment.'}
                  </p>
                  <Link
                    to="/pricing"
                    className="mt-4 inline-block text-body-sm font-medium text-accent hover:underline"
                  >
                    Start again from the pricing page
                  </Link>
                </div>
              ) : (
                <form
                  onSubmit={(e) => {
                    e.preventDefault()
                    void pay('success')
                  }}
                >
                  <p className="flex items-center gap-2 text-h3 text-text-primary">
                    <CreditCard className="h-5 w-5 text-text-secondary" aria-hidden="true" />
                    Card details
                  </p>
                  <p className="mt-1 text-caption text-text-tertiary">
                    Demo only — type anything. These fields are never sent.
                  </p>
                  <div className="mt-4 space-y-3">
                    <DemoField label="Name on card" placeholder="Ada Lovelace" />
                    <DemoField
                      label="Card number"
                      placeholder="4242 4242 4242 4242"
                      inputMode="numeric"
                    />
                    <div className="grid grid-cols-2 gap-3">
                      <DemoField label="Expiry" placeholder="12 / 29" />
                      <DemoField label="CVC" placeholder="123" inputMode="numeric" />
                    </div>
                  </div>

                  {error && (
                    <p
                      role="alert"
                      className="mt-4 rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-body-sm text-danger"
                    >
                      {error}
                    </p>
                  )}

                  <Button
                    type="submit"
                    size="lg"
                    className="mt-6 w-full"
                    loading={paying === 'success'}
                    disabled={paying !== null}
                  >
                    <Lock className="h-4 w-4" aria-hidden="true" />
                    Pay {formatPrice(sess.amount_cents)}
                  </Button>
                  <button
                    type="button"
                    onClick={() => void pay('declined')}
                    disabled={paying !== null}
                    className="mt-3 w-full text-center text-body-sm text-text-tertiary underline-offset-2 hover:text-text-secondary hover:underline disabled:opacity-50"
                  >
                    Simulate a declined card
                  </button>
                </form>
              )}
            </section>
          </div>
        )}
      </div>
    </div>
  )
}

/** An uncontrolled, unnamed input — nothing reads its value, and without a
 * `name` it can't be submitted anywhere even by accident. */
function DemoField({
  label,
  placeholder,
  inputMode,
}: {
  label: string
  placeholder: string
  inputMode?: 'numeric'
}) {
  return (
    <label className="block text-body-sm text-text-secondary">
      {label}
      <Input className="mt-1" placeholder={placeholder} autoComplete="off" inputMode={inputMode} />
    </label>
  )
}
