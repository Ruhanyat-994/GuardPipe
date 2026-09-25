import { type FormEvent, useEffect, useState } from 'react'
import { AlertTriangle, CheckCircle2, Clock, Mail, Send } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { ApiError } from '../lib/apiClient'
import {
  getNotificationSettings,
  resendReportEmailVerification,
  sendTestEmail,
  updateNotificationSettings,
  type NotificationSettings,
  type UpdateNotificationSettingsInput,
} from '../lib/notificationsApi'

function errorText(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.problem.detail : fallback
}

/**
 * `/settings/notifications` — the signed-in user's own scan-report email
 * settings: where reports go (their account email, or a verified other
 * address) and which scans send one. Personal, not org-wide, so every role
 * can reach it.
 */
export function NotificationSettingsPage() {
  const [settings, setSettings] = useState<NotificationSettings | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [emailDraft, setEmailDraft] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [message, setMessage] = useState<{ tone: 'success' | 'danger'; text: string } | null>(null)

  useEffect(() => {
    getNotificationSettings()
      .then((s) => {
        setSettings(s)
        setEmailDraft(s.using_account_email ? '' : s.report_email)
      })
      .catch((err: unknown) => setLoadError(errorText(err, 'Could not load your settings.')))
  }, [])

  async function save(input: UpdateNotificationSettingsInput, key: string, success?: string) {
    setBusy(key)
    setMessage(null)
    try {
      const next = await updateNotificationSettings(input)
      setSettings(next)
      if (success) setMessage({ tone: 'success', text: success })
    } catch (err) {
      setMessage({ tone: 'danger', text: errorText(err, 'Could not save your settings.') })
    } finally {
      setBusy(null)
    }
  }

  function handleEmailSubmit(e: FormEvent) {
    e.preventDefault()
    const value = emailDraft.trim()
    void save(
      { report_email: value },
      'email',
      value && value.toLowerCase() !== settings?.account_email.toLowerCase()
        ? `We sent a confirmation link to ${value}. Reports go there once you click it.`
        : 'Reports will go to your account email.',
    )
  }

  async function handleResend() {
    setBusy('resend')
    setMessage(null)
    try {
      await resendReportEmailVerification()
      setMessage({ tone: 'success', text: 'A new confirmation link is on its way.' })
    } catch (err) {
      setMessage({ tone: 'danger', text: errorText(err, 'Could not resend the link.') })
    } finally {
      setBusy(null)
    }
  }

  async function handleTest() {
    setBusy('test')
    setMessage(null)
    try {
      const res = await sendTestEmail()
      setMessage({ tone: 'success', text: `Test email sent to ${res.sent_to}.` })
    } catch (err) {
      setMessage({ tone: 'danger', text: errorText(err, 'Could not send a test email.') })
    } finally {
      setBusy(null)
    }
  }

  if (loadError) {
    return (
      <main className="mx-auto max-w-3xl px-6 py-8">
        <p role="alert" className="text-body-sm text-danger">
          {loadError}
        </p>
      </main>
    )
  }
  if (!settings) {
    return (
      <main className="mx-auto max-w-3xl px-6 py-8">
        <p className="text-body-sm text-text-secondary">Loading…</p>
      </main>
    )
  }

  const disabled = !settings.email_enabled

  return (
    <main className="mx-auto max-w-3xl px-6 py-8">
      <h1 className="mb-1 text-h1 text-text-primary">Notifications</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        When one of your scans finishes, GuardPipe tells you in the bell at the top of the page and
        can email you the PDF report.
      </p>

      {disabled && (
        <Card className="mb-4 border-warning/30 bg-warning/5">
          <p className="flex items-start gap-2 text-body-sm text-text-primary">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            Email isn&rsquo;t set up on this server, so reports can&rsquo;t be emailed. In-app
            notifications still work.
          </p>
        </Card>
      )}

      {message && (
        <p
          role={message.tone === 'danger' ? 'alert' : 'status'}
          className={`mb-4 text-body-sm ${message.tone === 'danger' ? 'text-danger' : 'text-success'}`}
        >
          {message.text}
        </p>
      )}

      <Card className="mb-4">
        <CardTitle className="flex items-center gap-2 text-h3">
          <Mail className="h-4 w-4" aria-hidden="true" />
          Where reports are sent
        </CardTitle>
        <CardDescription className="mt-1">
          Reports go to your account email ({settings.account_email}) unless you set a different
          address. A new address only starts receiving reports after you click the confirmation link
          we send to it.
        </CardDescription>

        <p className="mt-4 flex items-center gap-2 text-body-sm text-text-primary">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-success" aria-hidden="true" />
          Currently sending to <span className="font-semibold">{settings.report_email}</span>
          {settings.using_account_email && (
            <span className="text-text-tertiary">(account email)</span>
          )}
        </p>

        {settings.pending_email && (
          <div className="mt-3 flex flex-wrap items-center gap-3 rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-body-sm">
            <Clock className="h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <span className="min-w-0 flex-1">
              Waiting for <span className="font-semibold">{settings.pending_email}</span> to be
              confirmed.
            </span>
            <Button
              size="sm"
              variant="secondary"
              loading={busy === 'resend'}
              disabled={busy !== null || disabled}
              onClick={() => void handleResend()}
            >
              Resend link
            </Button>
          </div>
        )}

        <form onSubmit={handleEmailSubmit} className="mt-4 flex flex-wrap items-end gap-3">
          <label className="min-w-[240px] flex-1">
            <span className="mb-1 block text-body-sm font-medium text-text-primary">
              Report address
            </span>
            <Input
              type="email"
              value={emailDraft}
              onChange={(e) => setEmailDraft(e.target.value)}
              placeholder={`${settings.account_email} (account email)`}
              disabled={disabled}
              maxLength={254}
            />
          </label>
          <Button type="submit" loading={busy === 'email'} disabled={busy !== null || disabled}>
            Save address
          </Button>
        </form>
        <p className="mt-2 text-caption text-text-tertiary">
          Leave it empty to use your account email.
        </p>

        <div className="mt-4 border-t border-border-default pt-4">
          <Button
            size="sm"
            variant="secondary"
            loading={busy === 'test'}
            disabled={busy !== null || disabled}
            onClick={() => void handleTest()}
          >
            <Send className="h-3.5 w-3.5" aria-hidden="true" />
            Send a test email
          </Button>
        </div>
      </Card>

      <Card>
        <CardTitle className="text-h3">Which scans send an email</CardTitle>
        <CardDescription className="mt-1">
          You get an email for scans you start yourself, scans from schedules you created, and live
          scans from projects where you turned live scanning on.
        </CardDescription>
        <div className="mt-4 flex flex-col gap-3">
          <Toggle
            label="Manual and scheduled scans"
            description="Email the PDF report when the scan finishes or fails."
            checked={settings.email_on_scan_complete}
            disabled={busy !== null || disabled}
            onChange={(v) => void save({ email_on_scan_complete: v }, 'toggle')}
          />
          <Toggle
            label="Live scans (GitHub pushes and pull requests)"
            description="Off by default — every push can start a scan, which can mean a lot of email."
            checked={settings.email_on_live_scan}
            disabled={busy !== null || disabled}
            onChange={(v) => void save({ email_on_live_scan: v }, 'toggle')}
          />
        </div>
      </Card>
    </main>
  )
}

function Toggle({
  label,
  description,
  checked,
  disabled,
  onChange,
}: {
  label: string
  description: string
  checked: boolean
  disabled: boolean
  onChange: (value: boolean) => void
}) {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <input
        type="checkbox"
        className="mt-1 h-4 w-4 shrink-0 rounded border-border-strong accent-accent"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>
        <span className="block text-body-sm font-medium text-text-primary">{label}</span>
        <span className="block text-caption text-text-secondary">{description}</span>
      </span>
    </label>
  )
}
