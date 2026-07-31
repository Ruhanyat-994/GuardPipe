import { useEffect, useRef, useState, type ReactNode } from 'react'
import { cn } from '../../lib/cn'

/**
 * Headless open/close/outside-click/Escape primitive underlying every
 * overlay panel in the Jira-inspired app shell
 * (documentation/09-ui-ux-design-system.md §4.4) — `ContextMenu`,
 * `UserMenu`, `NotificationPanel`. Not itself a named component in that
 * spec; it's the shared plumbing so all four overlays behave identically
 * (click outside or Escape closes, only one open at a time per instance).
 */
export function Popover({
  trigger,
  children,
  align = 'right',
  panelClassName,
}: {
  trigger: (open: boolean, toggle: () => void) => ReactNode
  children: (close: () => void) => ReactNode
  align?: 'left' | 'right'
  panelClassName?: string
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function onPointerDown(e: PointerEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  return (
    <div className="relative" ref={ref}>
      {trigger(open, () => setOpen((o) => !o))}
      {open && (
        <div
          role="menu"
          className={cn(
            'absolute z-50 mt-2 min-w-[220px] rounded-md border border-border-default bg-bg-surface-raised shadow-lg',
            align === 'right' ? 'right-0' : 'left-0',
            panelClassName,
          )}
        >
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  )
}
