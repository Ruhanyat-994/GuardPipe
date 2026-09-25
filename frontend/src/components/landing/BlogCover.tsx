/**
 * Generated cover art for a blog post — the landing page's space motif
 * (a ringed planet, an orbit, a star field) in a per-category palette, so
 * posts get distinct, on-brand covers without any stock photography.
 * Deterministic: the same slug always draws the same picture.
 */

const PALETTES: Record<string, { bg: [string, string]; accent: string }> = {
  Product: { bg: ['#13244f', '#04060f'], accent: '#8fe3ff' },
  Security: { bg: ['#2a1450', '#07040f'], accent: '#c4a7ff' },
  Engineering: { bg: ['#0e3a3a', '#03090a'], accent: '#6ee7c8' },
  'Getting started': { bg: ['#1c2b5e', '#05070c'], accent: '#9db8ff' },
}

function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

function rng(seed: number) {
  let x = seed || 1
  return () => {
    x ^= x << 13
    x ^= x >>> 17
    x ^= x << 5
    return ((x >>> 0) % 10000) / 10000
  }
}

export function BlogCover({
  slug,
  category,
  className,
}: {
  slug: string
  category: string
  className?: string
}) {
  const p = PALETTES[category] ?? PALETTES['Getting started']
  const rand = rng(hash(slug))
  const id = `bc-${hash(slug).toString(36)}`
  // Posts in the same category still look distinct: a small hue shift each.
  const hue = Math.round(rand() * 44 - 22)
  const px = 120 + rand() * 160
  const py = 70 + rand() * 60
  const pr = 34 + rand() * 26
  const tilt = -25 + rand() * 20
  const stars = Array.from({ length: 38 }, () => ({
    x: rand() * 400,
    y: rand() * 225,
    r: rand() < 0.15 ? 1.2 : 0.6,
    o: 0.25 + rand() * 0.6,
  }))

  return (
    <svg
      viewBox="0 0 400 225"
      className={className}
      aria-hidden="true"
      preserveAspectRatio="xMidYMid slice"
      style={{ filter: `hue-rotate(${hue}deg)` }}
    >
      <defs>
        <radialGradient id={`${id}-bg`} cx="70%" cy="0%" r="120%">
          <stop offset="0%" stopColor={p.bg[0]} />
          <stop offset="100%" stopColor={p.bg[1]} />
        </radialGradient>
        <radialGradient id={`${id}-pl`} cx="35%" cy="30%" r="80%">
          <stop offset="0%" stopColor={p.accent} stopOpacity="0.9" />
          <stop offset="45%" stopColor={p.bg[0]} />
          <stop offset="100%" stopColor="#020308" />
        </radialGradient>
      </defs>
      <rect width="400" height="225" fill={`url(#${id}-bg)`} />
      {stars.map((s, i) => (
        <circle key={i} cx={s.x} cy={s.y} r={s.r} fill="#fff" opacity={s.o} />
      ))}
      <g transform={`rotate(${tilt} ${px} ${py})`}>
        <ellipse
          cx={px}
          cy={py}
          rx={pr * 2.3}
          ry={pr * 0.55}
          fill="none"
          stroke={p.accent}
          strokeOpacity="0.35"
          strokeWidth="1"
        />
      </g>
      <circle cx={px} cy={py} r={pr} fill={`url(#${id}-pl)`} />
      <g transform={`rotate(${tilt} ${px} ${py})`}>
        <path
          d={`M ${px - pr * 2.3} ${py} A ${pr * 2.3} ${pr * 0.55} 0 0 0 ${px + pr * 2.3} ${py}`}
          fill="none"
          stroke={p.accent}
          strokeWidth="1.6"
          strokeOpacity="0.8"
        />
      </g>
      <circle
        cx={px + pr * 2.3 * Math.cos(0.6)}
        cy={py + pr * 0.55 * Math.sin(0.6)}
        r="2.6"
        fill={p.accent}
      />
    </svg>
  )
}
