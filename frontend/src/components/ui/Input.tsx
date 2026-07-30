import { type InputHTMLAttributes, forwardRef } from 'react'
import { cn } from '../../lib/cn'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean
}

/** Domain-neutral primitive per documentation/09-ui-ux-design-system.md §4.1. */
export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { invalid = false, className, ...props },
  ref,
) {
  return (
    <input
      ref={ref}
      aria-invalid={invalid}
      className={cn(
        'h-10 w-full rounded-md border bg-bg-surface px-3 text-body text-text-primary',
        'placeholder:text-text-tertiary',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2',
        'disabled:opacity-50 disabled:pointer-events-none',
        invalid ? 'border-danger' : 'border-border-default',
        className,
      )}
      style={{ transitionDuration: 'var(--duration-fast)' }}
      {...props}
    />
  )
})
