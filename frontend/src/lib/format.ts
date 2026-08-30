/** Small formatting helpers shared across pages — no date library needed,
 * `Intl` already does this natively. */

const RTF = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const DIVISIONS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 60 * 60 * 24 * 365],
  ['month', 60 * 60 * 24 * 30],
  ['week', 60 * 60 * 24 * 7],
  ['day', 60 * 60 * 24],
  ['hour', 60 * 60],
  ['minute', 60],
]

const DATE_FORMAT = new Intl.DateTimeFormat('en', {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

/** Absolute timestamp — used where "2 days ago" isn't precise enough (an
 * audit trail, a suspension record) and the exact moment matters. */
export function formatDate(iso: string): string {
  return DATE_FORMAT.format(new Date(iso))
}

export function relativeTime(iso: string): string {
  const diffSec = Math.round((new Date(iso).getTime() - Date.now()) / 1000)
  for (const [unit, secondsInUnit] of DIVISIONS) {
    if (Math.abs(diffSec) >= secondsInUnit) {
      return RTF.format(Math.round(diffSec / secondsInUnit), unit)
    }
  }
  return RTF.format(diffSec, 'second')
}
