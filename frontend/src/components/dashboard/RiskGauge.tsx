/**
 * Hero element of the dashboard (documentation/09-ui-ux-design-system.md
 * §4.2): a 0–100 arc with the verdict word in its colour.
 */

export type Verdict = 'pass' | 'warn' | 'block'

const VERDICT_COLOR: Record<Verdict, string> = {
  pass: 'var(--success)',
  warn: 'var(--warning)',
  block: 'var(--danger)',
}

export function RiskGauge({
  score,
  verdict,
  size = 168,
}: {
  score: number
  verdict: Verdict
  size?: number
}) {
  const stroke = 14
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const progress = Math.min(Math.max(score, 0), 100) / 100
  const color = VERDICT_COLOR[verdict]

  return (
    <div
      className="relative flex items-center justify-center"
      style={{ width: size, height: size }}
    >
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="-rotate-90">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="var(--border-default)"
          strokeWidth={stroke}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={`${circumference * progress} ${circumference}`}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-display text-text-primary">{score}</span>
        <span className="text-caption font-semibold tracking-wide uppercase" style={{ color }}>
          {verdict}
        </span>
      </div>
    </div>
  )
}
