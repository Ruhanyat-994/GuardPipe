import { cn } from '../../lib/cn'

export interface TabItem {
  id: string
  label: string
}

/**
 * Domain-neutral primitive per documentation/09-ui-ux-design-system.md §4.1
 * — a small controlled tab switcher. No internal state: the caller owns
 * `active`, same pattern as every other primitive in this directory.
 */
export function Tabs({
  items,
  active,
  onChange,
  className,
}: {
  items: TabItem[]
  active: string
  onChange: (id: string) => void
  className?: string
}) {
  return (
    <div
      role="tablist"
      className={cn('flex items-center gap-1 border-b border-border-default', className)}
    >
      {items.map((item) => {
        const isActive = item.id === active
        return (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange(item.id)}
            className={cn(
              '-mb-px border-b-2 px-3 py-1.5 text-body-sm font-medium transition-colors',
              isActive
                ? 'border-accent text-accent'
                : 'border-transparent text-text-secondary hover:text-text-primary',
            )}
          >
            {item.label}
          </button>
        )
      })}
    </div>
  )
}
