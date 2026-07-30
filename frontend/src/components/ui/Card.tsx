import { type HTMLAttributes } from 'react'
import { cn } from '../../lib/cn'

/**
 * Domain-neutral primitive per documentation/09-ui-ux-design-system.md §4.1.
 * Inside-card padding is `space-6` (24px) and cards use `shadow-sm` per §2.6.
 */
export function Card({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'rounded-lg border border-border-default bg-bg-surface p-6 shadow-sm',
        className,
      )}
      {...props}
    />
  )
}

export function CardTitle({ className, ...props }: HTMLAttributes<HTMLHeadingElement>) {
  return <h2 className={cn('text-h2 text-text-primary', className)} {...props} />
}

export function CardDescription({ className, ...props }: HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn('text-body text-text-secondary', className)} {...props} />
}
