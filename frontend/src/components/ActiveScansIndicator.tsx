import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Popover } from './ui/Popover'
import { useActiveScansStore } from '../stores/activeScansStore'

/**
 * Top-bar "N scans running" chip. Scans run server-side no matter which
 * page is open; this makes that visible — a user can start a scan, go do
 * something else, and still see it progress from any page (and get a toast
 * from activeScansStore when it finishes). Renders nothing while no scan
 * is in flight.
 */
export function ActiveScansIndicator() {
  const scans = useActiveScansStore((s) => s.scans)
  const progress = useActiveScansStore((s) => s.progress)
  const start = useActiveScansStore((s) => s.start)
  const stop = useActiveScansStore((s) => s.stop)

  useEffect(() => {
    start()
    return stop
  }, [start, stop])

  if (scans.length === 0) return null

  return (
    <Popover
      panelClassName="w-80"
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={toggle}
          aria-haspopup="menu"
          aria-expanded={open}
          className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-body-sm text-chrome-text-secondary hover:bg-chrome-hover hover:text-chrome-text"
        >
          <Loader2 className="h-4 w-4 animate-spin text-accent" aria-hidden="true" />
          {scans.length} {scans.length === 1 ? 'scan' : 'scans'} running
        </button>
      )}
    >
      {(close) => (
        <div className="max-h-96 overflow-y-auto py-1">
          <p className="px-4 pt-2 pb-1 text-caption text-text-tertiary">
            Scans keep running while you use the rest of GuardPipe. You&rsquo;ll be notified when
            each one finishes.
          </p>
          <ul className="flex flex-col divide-y divide-border-default">
            {scans.map((scan) => {
              const pct = progress[scan.id]
              return (
                <li key={scan.id}>
                  <Link
                    to={`/scans/${scan.id}`}
                    onClick={close}
                    className="flex flex-col gap-1.5 px-4 py-2.5 hover:bg-bg-subtle"
                  >
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="min-w-0 truncate text-body-sm font-medium text-text-primary">
                        {scan.project_name} · Scan #{scan.scan_number}
                      </span>
                      <span className="shrink-0 text-caption text-text-tertiary">
                        {scan.status === 'queued' ? 'Queued' : pct !== undefined ? `${pct}%` : ''}
                      </span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-bg-subtle">
                      <div
                        className="h-full rounded-full bg-accent transition-[width]"
                        style={{ width: `${scan.status === 'queued' ? 0 : (pct ?? 0)}%` }}
                      />
                    </div>
                  </Link>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </Popover>
  )
}
