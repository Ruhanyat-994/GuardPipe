import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '../../lib/cn'

/**
 * icon + title + description + optional action
 * (documentation/09-ui-ux-design-system.md §4.2) — the calm, illustrated
 * alternative to dumping a raw status string as plain text. Used anywhere
 * "there's nothing here" is itself the honest, correct outcome (an engine
 * skipped because its target doesn't exist in this project) rather than a
 * failure — tone defaults to neutral, never the alarming red/orange
 * treatment reserved for genuine failures (principle 7, "calm, not
 * alarming").
 */
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  tone = 'neutral',
  size = 'default',
  className,
}: {
  icon: LucideIcon
  title: string
  description?: string
  action?: ReactNode
  tone?: 'neutral' | 'warning' | 'danger'
  size?: 'default' | 'compact'
  className?: string
}) {
  const toneColor =
    tone === 'danger'
      ? 'var(--danger)'
      : tone === 'warning'
        ? 'var(--warning)'
        : 'var(--text-tertiary)'

  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center text-center',
        size === 'compact' ? 'gap-2 py-6' : 'gap-3 py-10',
        className,
      )}
    >
      <div
        className={cn(
          'flex items-center justify-center rounded-full',
          size === 'compact' ? 'h-9 w-9' : 'h-12 w-12',
        )}
        style={{
          backgroundColor: `color-mix(in srgb, ${toneColor} 12%, transparent)`,
          color: toneColor,
        }}
      >
        <Icon className={size === 'compact' ? 'h-4.5 w-4.5' : 'h-6 w-6'} aria-hidden="true" />
      </div>
      <div className="flex flex-col gap-1">
        <p
          className={cn(
            'font-semibold text-text-primary',
            size === 'compact' ? 'text-body-sm' : 'text-body',
          )}
        >
          {title}
        </p>
        {description && (
          <p
            className={cn(
              'max-w-sm text-text-secondary',
              size === 'compact' ? 'text-caption' : 'text-body-sm',
            )}
          >
            {description}
          </p>
        )}
      </div>
      {action}
    </div>
  )
}
