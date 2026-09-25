import { useEffect, useRef, useState } from 'react'

/** Animates a number towards `target` over `durationMs` (ease-out), so a
 * balance visibly counts down/up instead of jumping. With
 * prefers-reduced-motion it lands on the value in a single frame. */
export function useCountUp(target: number, durationMs = 600): number {
  const [value, setValue] = useState(target)
  const current = useRef(target)

  useEffect(() => {
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    const duration = reduce ? 0 : durationMs
    const from = current.current
    const start = performance.now()
    let frame = 0
    const tick = (now: number) => {
      const t = duration <= 0 ? 1 : Math.min(1, (now - start) / duration)
      const eased = 1 - Math.pow(1 - t, 3)
      current.current = Math.round(from + (target - from) * eased)
      setValue(current.current)
      if (t < 1) frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [target, durationMs])

  return value
}
