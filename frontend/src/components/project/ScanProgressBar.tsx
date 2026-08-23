import { Logo } from '../Logo'
import { cn } from '../../lib/cn'

/**
 * A real, live percentage bar — not a fixed/fabricated fill. Every caller
 * passes a genuine number: for pentest, the engine's own current phase
 * (internal/engines/pentest/engine.go's ReportProgress calls); for every
 * other engine, a real elapsed-time-against-timeout estimate
 * (orchestrator's own doc comments on that fallback); at the whole-scan
 * level, the mean of every job's own real pct (orchestrator.Service.GetProgress),
 * not a "jobs fully done" count — that measure sits frozen at 0% for a
 * scan's entire running time when it only has one job, then jumps straight
 * to 100. The GuardPipe shield badge slides left-to-right with the fill's
 * actual leading edge, not fixed at either end.
 */
export function ScanProgressBar({
  pct,
  size = 'md',
  className,
}: {
  pct: number
  size?: 'sm' | 'md'
  className?: string
}) {
  const clamped = Math.max(0, Math.min(100, Math.round(pct)))
  const trackHeight = size === 'md' ? 'h-7' : 'h-4'
  const badgeSize = size === 'md' ? 'h-8 w-8' : 'h-5 w-5'
  const logoSize = size === 'md' ? 'h-4.5 w-4.5' : 'h-2.5 w-2.5'

  return (
    <div className={cn('flex items-center', className)}>
      <div className="relative flex-1 py-1">
        <div
          className={cn(
            'overflow-hidden rounded-full ring-1 ring-inset ring-white/10',
            trackHeight,
          )}
          style={{ backgroundColor: '#0b1220' }}
          role="progressbar"
          aria-valuenow={clamped}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className="h-full rounded-full transition-[width] duration-700 ease-out"
            style={{
              width: `${clamped}%`,
              backgroundImage: 'linear-gradient(90deg, #1d4ed8 0%, #2563eb 55%, #60a5fa 100%)',
            }}
          />
        </div>
        {/* The badge tracks the fill's real leading edge — left: {pct}% —
            transitioning on the same schedule as the fill itself so it
            visibly glides left to right as real progress comes in, rather
            than sitting fixed at either end. Positioned outside the
            track's own overflow-hidden div so it's never clipped and can
            slightly overhang the track edges, same as the reference. */}
        <div
          className={cn(
            'absolute top-1/2 z-[1] flex items-center justify-center rounded-full transition-[left] duration-700 ease-out',
            badgeSize,
          )}
          style={{
            left: `${clamped}%`,
            transform: 'translate(-50%, -50%)',
            backgroundColor: '#0b1220',
            boxShadow: '0 0 0 2px var(--accent)',
          }}
        >
          <Logo className={logoSize} />
        </div>
      </div>
      <span
        className={cn(
          'ml-3 shrink-0 text-right font-bold tabular-nums text-text-primary',
          size === 'md' ? 'w-11 text-body-sm' : 'w-8 text-caption',
        )}
      >
        {clamped}%
      </span>
    </div>
  )
}
