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

export function relativeTime(iso: string): string {
  const diffSec = Math.round((new Date(iso).getTime() - Date.now()) / 1000)
  for (const [unit, secondsInUnit] of DIVISIONS) {
    if (Math.abs(diffSec) >= secondsInUnit) {
      return RTF.format(Math.round(diffSec / secondsInUnit), unit)
    }
  }
  return RTF.format(diffSec, 'second')
}
