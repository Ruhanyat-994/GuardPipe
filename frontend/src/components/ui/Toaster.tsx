import { Link } from 'react-router-dom'
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react'
import { cn } from '../../lib/cn'
import { useToastStore, type ToastTone } from '../../stores/toastStore'

const TONE_ICON: Record<ToastTone, typeof Info> = {
  success: CheckCircle2,
  warning: AlertTriangle,
  danger: XCircle,
  info: Info,
}

const TONE_CLASS: Record<ToastTone, string> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  info: 'text-accent',
}

/** Bottom-right stack of transient notices (toastStore). Rendered once, by
 * AppShell. `aria-live` so a screen reader hears a scan finishing without
 * the user having to go and look. */
export function Toaster() {
  const toasts = useToastStore((s) => s.toasts)
  const dismiss = useToastStore((s) => s.dismiss)

  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed right-4 bottom-4 z-[60] flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2"
    >
      {toasts.map((t) => {
        const Icon = TONE_ICON[t.tone]
        return (
          <div
            key={t.id}
            role="status"
            className="pointer-events-auto flex items-start gap-3 rounded-md border border-border-default bg-bg-surface-raised p-3 shadow-lg"
          >
            <Icon
              className={cn('mt-0.5 h-5 w-5 shrink-0', TONE_CLASS[t.tone])}
              aria-hidden="true"
            />
            <div className="min-w-0 flex-1">
              <p className="text-body-sm font-semibold text-text-primary">{t.title}</p>
              {t.body && <p className="text-caption break-words text-text-secondary">{t.body}</p>}
              {t.href && (
                <Link
                  to={t.href}
                  onClick={() => dismiss(t.id)}
                  className="mt-1 inline-block text-caption font-medium text-accent hover:underline"
                >
                  {t.hrefLabel ?? 'View'}
                </Link>
              )}
            </div>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              className="rounded p-0.5 text-text-tertiary hover:text-text-primary"
              aria-label="Dismiss"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        )
      })}
    </div>
  )
}
