import { Bell, Inbox } from 'lucide-react'
import { Popover } from './ui/Popover'

/**
 * documentation/09-ui-ux-design-system.md §4.4 — slide-in panel anchored
 * under the bell icon. There is no notification-producing backend yet
 * (that's the orchestrator, Phase 6, plus a real notifications table this
 * project doesn't have), so this ships as a genuine, honest empty state
 * rather than fabricated sample notifications — the same "never fake a
 * result" discipline the rest of the product holds to.
 */
export function NotificationPanel() {
  return (
    <Popover
      panelClassName="w-80"
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={toggle}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label="Notifications"
          className="rounded-md p-2 text-chrome-text-secondary hover:bg-chrome-hover hover:text-chrome-text"
        >
          <Bell className="h-5 w-5" aria-hidden="true" />
        </button>
      )}
    >
      {() => (
        <div className="flex flex-col items-center gap-2 px-6 py-10 text-center">
          <Inbox className="h-8 w-8 text-text-tertiary" aria-hidden="true" />
          <p className="text-body-sm font-medium text-text-primary">You&apos;re all caught up</p>
          <p className="text-caption text-text-tertiary">
            Scan-completion and finding alerts land here once the orchestrator (Phase 6) exists.
          </p>
        </div>
      )}
    </Popover>
  )
}
