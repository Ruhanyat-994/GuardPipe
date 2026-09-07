import { useEffect, useRef, useState } from 'react'
import { Bell, Building2, Check, FolderKanban, Inbox, X } from 'lucide-react'
import { Popover } from './ui/Popover'
import { Button } from './ui/Button'
import { ApiError } from '../lib/apiClient'
import {
  acceptInvite,
  declineInvite,
  listMyInvites,
  type PendingInvite,
} from '../lib/organizationApi'
import {
  acceptProjectInvite,
  declineProjectInvite,
  listMyProjectInvites,
  type PendingProjectInvite,
} from '../lib/projectsApi'

/** How often the bell polls for new invites while the app is open — there's
 * no push/WebSocket channel in this codebase (Redis is job-queue only), so
 * this is what "live" means here: near-real-time via a cheap, indexed
 * query, the same tradeoff `orchestrator.GetProgress`'s 2s scan-progress
 * poll already makes for a much higher-frequency event. Invites are rare
 * enough that 20s is "live" for this purpose without hammering the API. */
const POLL_INTERVAL_MS = 20_000

/** A merged, sorted feed item — either kind of invite renders through the
 * same Accept/Decline UI, tagged just enough to know which API and copy to
 * use. project-collaborators follow-up: previously this panel only ever
 * showed org invites. */
type FeedItem =
  { kind: 'org'; invite: PendingInvite } | { kind: 'project'; invite: PendingProjectInvite }

/**
 * documentation/09-ui-ux-design-system.md §4.4 — slide-in panel anchored
 * under the bell icon. Previously a genuine, honest empty state (no
 * notification-producing backend existed yet); this is that backend's
 * first real event, added 2026-09-02 as a Phase 15 follow-up: an org invite
 * to an already-registered account now surfaces here directly — Accept or
 * Decline right from the panel — rather than only via a link someone has to
 * copy and paste. Extended by the project-collaborators follow-up to also
 * poll project-scoped invites (`GET /project-invites/mine`) into the same
 * feed. Scan-completion/finding alerts (the event this component was
 * originally scoped for) still don't exist and still aren't faked here.
 */
export function NotificationPanel() {
  const [items, setItems] = useState<FeedItem[]>([])
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busyKey, setBusyKey] = useState<string | null>(null)

  const pollRef = useRef<number | null>(null)

  function refresh() {
    Promise.all([listMyInvites(), listMyProjectInvites()])
      .then(([orgRes, projectRes]) => {
        const merged: FeedItem[] = [
          ...orgRes.data.map((invite): FeedItem => ({ kind: 'org', invite })),
          ...projectRes.data.map((invite): FeedItem => ({ kind: 'project', invite })),
        ].sort(
          (a, b) =>
            new Date(b.invite.created_at).getTime() - new Date(a.invite.created_at).getTime(),
        )
        setItems(merged)
        setError(null)
      })
      .catch(() => {
        // A failed background poll shouldn't nag the user — the bell just
        // quietly keeps whatever it last knew, same as a missed heartbeat.
      })
      .finally(() => setLoaded(true))
  }

  useEffect(() => {
    refresh()
    pollRef.current = window.setInterval(refresh, POLL_INTERVAL_MS)
    return () => {
      if (pollRef.current !== null) window.clearInterval(pollRef.current)
    }
  }, [])

  function key(item: FeedItem): string {
    return `${item.kind}:${item.invite.id}`
  }

  async function handleAccept(item: FeedItem) {
    setBusyKey(key(item))
    setError(null)
    try {
      if (item.kind === 'org') {
        await acceptInvite(item.invite.id)
      } else {
        await acceptProjectInvite(item.invite.id)
      }
      setItems((prev) => prev.filter((i) => key(i) !== key(item)))
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not accept this invite.')
    } finally {
      setBusyKey(null)
    }
  }

  async function handleDecline(item: FeedItem) {
    setBusyKey(key(item))
    setError(null)
    try {
      if (item.kind === 'org') {
        await declineInvite(item.invite.id)
      } else {
        await declineProjectInvite(item.invite.id)
      }
      setItems((prev) => prev.filter((i) => key(i) !== key(item)))
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not decline this invite.')
    } finally {
      setBusyKey(null)
    }
  }

  const unreadCount = items.length

  return (
    <Popover
      panelClassName="w-80"
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={() => {
            // Popover has no open-event hook of its own — refresh right
            // before opening so the panel never shows a stale list from up
            // to POLL_INTERVAL_MS ago at the exact moment someone checks it.
            if (!open) refresh()
            toggle()
          }}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label={unreadCount > 0 ? `Notifications (${unreadCount} unread)` : 'Notifications'}
          className="relative rounded-md p-2 text-chrome-text-secondary hover:bg-chrome-hover hover:text-chrome-text"
        >
          <Bell className="h-5 w-5" aria-hidden="true" />
          {unreadCount > 0 && (
            <span
              className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-accent px-1 text-[10px] font-semibold leading-none text-text-inverse"
              aria-hidden="true"
            >
              {unreadCount}
            </span>
          )}
        </button>
      )}
    >
      {() => (
        <div className="max-h-96 overflow-y-auto py-1">
          {error && (
            <p role="alert" className="px-4 py-2 text-caption text-danger">
              {error}
            </p>
          )}

          {items.length > 0 ? (
            <ul className="flex flex-col divide-y divide-border-default">
              {items.map((item) => (
                <li key={key(item)} className="flex flex-col gap-2 px-4 py-3">
                  <div className="flex items-start gap-2.5">
                    {item.kind === 'org' ? (
                      <Building2
                        className="mt-0.5 h-4 w-4 shrink-0 text-text-tertiary"
                        aria-hidden="true"
                      />
                    ) : (
                      <FolderKanban
                        className="mt-0.5 h-4 w-4 shrink-0 text-text-tertiary"
                        aria-hidden="true"
                      />
                    )}
                    {item.kind === 'org' ? (
                      <p className="text-body-sm text-text-primary">
                        You&rsquo;ve been invited to join{' '}
                        <span className="font-semibold">{item.invite.org_name}</span> as{' '}
                        <span className="capitalize">{item.invite.role}</span>.
                      </p>
                    ) : (
                      <p className="text-body-sm text-text-primary">
                        You&rsquo;ve been invited to collaborate on{' '}
                        <span className="font-semibold">{item.invite.project_name}</span> (shared by{' '}
                        {item.invite.org_name}) as{' '}
                        <span className="capitalize">{item.invite.role}</span>.
                      </p>
                    )}
                  </div>
                  <div className="flex gap-2 pl-6">
                    <Button
                      size="sm"
                      loading={busyKey === key(item)}
                      onClick={() => void handleAccept(item)}
                    >
                      <Check className="h-3.5 w-3.5" aria-hidden="true" />
                      Accept
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busyKey !== null}
                      onClick={() => void handleDecline(item)}
                    >
                      <X className="h-3.5 w-3.5" aria-hidden="true" />
                      Decline
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <div className="flex flex-col items-center gap-2 px-6 py-10 text-center">
              <Inbox className="h-8 w-8 text-text-tertiary" aria-hidden="true" />
              <p className="text-body-sm font-medium text-text-primary">
                {loaded ? "You're all caught up" : 'Loading…'}
              </p>
              <p className="text-caption text-text-tertiary">
                Org and project invites land here live. Scan-completion and finding alerts will too,
                once the orchestrator produces them.
              </p>
            </div>
          )}
        </div>
      )}
    </Popover>
  )
}
