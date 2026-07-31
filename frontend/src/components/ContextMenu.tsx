import type { LucideIcon } from 'lucide-react'
import { MoreHorizontal } from 'lucide-react'
import { Popover } from './ui/Popover'
import { cn } from '../lib/cn'

/**
 * Per-object `···` menu (documentation/09-ui-ux-design-system.md §4.4):
 * grouped rows, primary actions first, destructive actions in `--danger`
 * at the bottom, separated by a divider — same visual grammar as Jira's
 * space/board context menus.
 */
export interface ContextMenuItem {
  label: string
  icon?: LucideIcon
  onClick: () => void
  destructive?: boolean
  disabled?: boolean
}

export function ContextMenu({
  groups,
  align = 'right',
  label = 'Actions',
}: {
  groups: ContextMenuItem[][]
  align?: 'left' | 'right'
  label?: string
}) {
  return (
    <Popover
      align={align}
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={toggle}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label={label}
          className="rounded-md p-1.5 text-text-secondary hover:bg-bg-subtle hover:text-text-primary"
        >
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </button>
      )}
    >
      {(close) => (
        <div className="py-1">
          {groups.map((group, i) => (
            <div key={i} className={i > 0 ? 'border-t border-border-default py-1' : 'py-1'}>
              {group.map((item) => (
                <button
                  key={item.label}
                  type="button"
                  role="menuitem"
                  disabled={item.disabled}
                  onClick={() => {
                    close()
                    item.onClick()
                  }}
                  className={cn(
                    'flex w-full items-center gap-2.5 px-3 py-2 text-left text-body-sm hover:bg-bg-subtle disabled:cursor-not-allowed disabled:opacity-50',
                    item.destructive ? 'text-danger' : 'text-text-primary',
                  )}
                >
                  {item.icon && <item.icon className="h-4 w-4 shrink-0" aria-hidden="true" />}
                  {item.label}
                </button>
              ))}
            </div>
          ))}
        </div>
      )}
    </Popover>
  )
}
