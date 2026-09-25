import { type RefObject, useEffect, useRef } from 'react'
import logoSrc from '../../assets/logo.png'

/**
 * The hero's motion graphic. Insecure input arrives on the left along
 * several branch-shaped pipes — forking and merging like a git network
 * graph, each a different thickness, tinted red — carrying real-looking
 * vulnerabilities (a leaked key, a vulnerable package, string-built SQL, a
 * privileged pod, a curl-to-shell step). Everything funnels into GuardPipe,
 * the middleman. On the right the same branching shape fans back out in
 * blue, each branch carrying the fixed version of what came in on its
 * mirror lane, while a chip above the middleman names each fix.
 *
 * Text flows along the pipes via <textPath>, moved by one requestAnimationFrame
 * loop through refs (no React re-render per frame).
 * Pauses off-screen / hidden tab; reduced motion renders one still frame.
 */

const W = 1600
const H = 420
const NODE_X = 800
const NODE_Y = 238
const IN_END = NODE_X - 92 // where the inflow lanes meet the pill
const OUT_START = NODE_X + 92 // where the outflow lanes leave it

interface Lane {
  d: string
  width: number
  font: number
  color: string
  text: string
  speed: number
}

const RED_STROKE = 'rgba(248,113,113,0.38)'
const BLUE_STROKE = 'rgba(111,147,255,0.55)'

// Inflow — git-graph lanes (horizontal runs joined by rounded elbows), all
// converging on the pill. Deliberately different stroke and text sizes.
const IN_LANES: Lane[] = [
  {
    d: `M -40 70 H 250 C 330 70 330 150 410 150 H 560 C 650 150 660 ${NODE_Y} ${IN_END} ${NODE_Y}`,
    width: 1,
    font: 10.5,
    color: 'rgba(252,165,165,0.8)',
    text: 'AWS_SECRET_ACCESS_KEY=••••••••••••  ·  password = "hunter2"  ·  -----BEGIN RSA PRIVATE KEY-----  ·  ',
    speed: 0.055,
  },
  {
    d: `M -40 160 H 120 C 190 160 190 110 260 110 H 330`,
    width: 0.8,
    font: 9,
    color: 'rgba(252,165,165,0.55)',
    text: 'fork: feature/login  ·  ',
    speed: 0.035,
  },
  {
    d: `M -40 ${NODE_Y} H ${IN_END}`,
    width: 1.8,
    font: 13,
    color: 'rgba(252,165,165,0.9)',
    text: 'query = "SELECT * FROM users WHERE id=" + req.id  ·  eval(req.body.code)  ·  ',
    speed: 0.07,
  },
  {
    d: `M -40 330 H 170 C 250 330 250 290 330 290 H 580 C 660 290 660 ${NODE_Y} ${IN_END} ${NODE_Y}`,
    width: 1.3,
    font: 11.5,
    color: 'rgba(253,164,175,0.85)',
    text: 'lodash@4.17.15 (CVE-2020-8203)  ·  log4j-core 2.14.1  ·  requests==2.19.0  ·  ',
    speed: 0.06,
  },
  {
    d: `M -40 400 H 300 C 390 400 390 350 470 350 H 600 C 690 350 680 ${NODE_Y} ${IN_END} ${NODE_Y}`,
    width: 1,
    font: 10,
    color: 'rgba(252,165,165,0.75)',
    text: 'privileged: true  ·  hostNetwork: true  ·  uses: some/action@main  ·  curl https://get.sh | bash  ·  ',
    speed: 0.05,
  },
]

// Outflow — the same branching shape, mirrored and leaving the pill, in
// blue. Each lane carries the fix for its mirror inflow lane. Paths start
// at the pill so text flows *away* from it.
const X = (x: number) => 2 * NODE_X - x // mirror a left-side x to the right
const OUT_LANES: Lane[] = [
  {
    d: `M ${OUT_START} ${NODE_Y} C ${X(660)} ${NODE_Y} ${X(650)} 150 ${X(560)} 150 H ${X(410)} C ${X(330)} 150 ${X(330)} 70 ${X(250)} 70 H ${W + 40}`,
    width: 1,
    font: 10.5,
    color: 'rgba(143,227,255,0.9)',
    text: 'secret moved to a vault  ·  key rotated and revoked  ·  password removed from source  ·  ',
    speed: 0.055,
  },
  {
    d: `M ${X(330)} 110 H ${X(260)} C ${X(190)} 110 ${X(190)} 160 ${X(120)} 160 H ${W + 40}`,
    width: 0.8,
    font: 9,
    color: 'rgba(157,184,255,0.6)',
    text: 'merged: main ✓  ·  ',
    speed: 0.035,
  },
  {
    d: `M ${OUT_START} ${NODE_Y} H ${W + 40}`,
    width: 1.8,
    font: 13,
    color: 'rgba(201,216,255,0.95)',
    text: 'db.query("SELECT * FROM users WHERE id = $1", [req.id])  ·  no eval  ·  ',
    speed: 0.07,
  },
  {
    d: `M ${OUT_START} ${NODE_Y} C ${X(660)} ${NODE_Y} ${X(660)} 290 ${X(580)} 290 H ${X(330)} C ${X(250)} 290 ${X(250)} 330 ${X(170)} 330 H ${W + 40}`,
    width: 1.3,
    font: 11.5,
    color: 'rgba(143,227,255,0.85)',
    text: 'lodash@4.17.21  ·  log4j-core 2.17.1  ·  requests==2.31.0  ·  ',
    speed: 0.06,
  },
  {
    d: `M ${OUT_START} ${NODE_Y} C ${X(680)} ${NODE_Y} ${X(690)} 350 ${X(600)} 350 H ${X(470)} C ${X(390)} 350 ${X(390)} 400 ${X(300)} 400 H ${W + 40}`,
    width: 1,
    font: 10,
    color: 'rgba(157,184,255,0.8)',
    text: 'privileged: false  ·  runAsNonRoot: true  ·  uses: some/action@8f4b7f8  ·  no curl | bash  ·  ',
    speed: 0.05,
  },
]

const IN_COMMITS = [
  [90, 70],
  [250, 70],
  [410, 150],
  [120, 160],
  [330, 110],
  [200, NODE_Y],
  [420, NODE_Y],
  [170, 330],
  [330, 290],
  [300, 400],
  [470, 350],
]
const OUT_COMMITS = IN_COMMITS.map(([x, y]) => [X(x), y])

const FIXES = [
  'Secret removed',
  'CVE patched',
  'SQL injection fixed',
  'Privilege dropped',
  'Action pinned',
]
const FIX_EVERY = 1.9 // seconds
const BARS = 13

function Lanes({
  lanes,
  prefix,
  stroke,
  commits,
  commitStroke,
  mask,
  textRefs,
}: {
  lanes: Lane[]
  prefix: string
  stroke: string
  commits: number[][]
  commitStroke: string
  mask: string
  textRefs: RefObject<(SVGTextPathElement | null)[]>
}) {
  return (
    <g mask={`url(#${mask})`}>
      {lanes.map((l, i) => (
        <use key={i} href={`#${prefix}-${i}`} fill="none" stroke={stroke} strokeWidth={l.width} />
      ))}
      {commits.map(([x, y]) => (
        <circle
          key={`${x}-${y}`}
          cx={x}
          cy={y}
          r={3.2}
          fill="#05070c"
          stroke={commitStroke}
          strokeWidth={1}
        />
      ))}
      {lanes.map((l, i) => (
        <text
          key={i}
          fontSize={l.font}
          fill={l.color}
          fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace"
          dy={-5}
        >
          <textPath
            ref={(el) => {
              textRefs.current[i] = el
            }}
            href={`#${prefix}-${i}`}
          >
            {l.text.repeat(3)}
          </textPath>
        </text>
      ))}
    </g>
  )
}

export function SecurePipeFlow() {
  const svgRef = useRef<SVGSVGElement>(null)
  const inTextRefs = useRef<(SVGTextPathElement | null)[]>([])
  const outTextRefs = useRef<(SVGTextPathElement | null)[]>([])
  const chipRef = useRef<SVGGElement>(null)
  const chipTextRef = useRef<SVGTextElement>(null)
  const chipBgRef = useRef<SVGRectElement>(null)
  const checkRef = useRef<SVGPathElement>(null)
  const barRefs = useRef<(SVGRectElement | null)[]>([])
  const pulseRef = useRef<SVGCircleElement>(null)

  useEffect(() => {
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    // Each string is repeated 3x in the DOM; one repetition's width is the
    // loop length, so shifting by exactly that is seamless.
    const repWidth = (el: SVGTextPathElement | null) => {
      const t = el?.parentElement as unknown as SVGTextContentElement | null
      const w = t ? t.getComputedTextLength() / 3 : 0
      return w > 0 ? w : 400
    }
    const streams = [
      ...IN_LANES.map((l, i) => ({ lane: l, el: () => inTextRefs.current[i] })),
      ...OUT_LANES.map((l, i) => ({ lane: l, el: () => outTextRefs.current[i] })),
    ].map((s, i) => {
      const rep = repWidth(s.el())
      return { ...s, rep, offset: -rep * ((i * 0.37) % 1) }
    })
    let clock = 0
    let lastFix = -1
    let raf = 0
    let last = performance.now()

    function setChip(i: number) {
      const label = FIXES[((i % FIXES.length) + FIXES.length) % FIXES.length]
      const w = 40 + label.length * 7.4
      if (chipTextRef.current) {
        chipTextRef.current.textContent = label
        chipTextRef.current.setAttribute('x', String(NODE_X - w / 2 + 30))
      }
      chipBgRef.current?.setAttribute('width', String(w))
      chipBgRef.current?.setAttribute('x', String(NODE_X - w / 2))
      checkRef.current?.setAttribute('transform', `translate(${NODE_X - w / 2 + 14} 0)`)
    }

    function render(dt: number) {
      for (const s of streams) {
        s.offset += s.lane.speed * dt
        if (s.offset > 0) s.offset -= s.rep
        s.el()?.setAttribute('startOffset', s.offset.toFixed(1))
      }

      barRefs.current.forEach((bar, k) => {
        if (!bar) return
        const h =
          6 + Math.abs(Math.sin(clock * (3.2 + k * 0.37) + k)) * 20 * (0.6 + 0.4 * Math.sin(k))
        bar.setAttribute('height', h.toFixed(1))
        bar.setAttribute('y', (NODE_Y - h / 2).toFixed(1))
      })

      const fix = Math.floor(clock / FIX_EVERY)
      if (fix !== lastFix) {
        lastFix = fix
        setChip(fix)
      }
      const phase = (clock % FIX_EVERY) / FIX_EVERY
      if (chipRef.current) {
        const pop = phase < 0.12 ? phase / 0.12 : phase > 0.85 ? (1 - phase) / 0.15 : 1
        chipRef.current.style.opacity = String(pop)
        chipRef.current.setAttribute(
          'transform',
          `translate(0 ${(phase < 0.12 ? (1 - pop) * 8 : 0).toFixed(1)})`,
        )
      }
      if (pulseRef.current) {
        const k = Math.min(1, phase / 0.35)
        pulseRef.current.setAttribute('r', String(40 + k * 60))
        pulseRef.current.style.opacity = String(0.5 * (1 - k))
      }
    }

    function loop(now: number) {
      // The first frame can be timestamped slightly before `last`; never
      // let time run backwards.
      const dtMs = Math.max(0, Math.min(64, now - last))
      last = now
      clock += dtMs / 1000
      render(dtMs)
      raf = requestAnimationFrame(loop)
    }

    render(0)
    setChip(0)
    if (reduced) {
      if (chipRef.current) chipRef.current.style.opacity = '1'
      return
    }

    let visible = true
    const start = () => {
      if (raf || !visible || document.hidden) return
      last = performance.now()
      raf = requestAnimationFrame(loop)
    }
    const stop = () => {
      cancelAnimationFrame(raf)
      raf = 0
    }
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting
      if (visible) start()
      else stop()
    })
    if (svgRef.current) observer.observe(svgRef.current)
    const onVisibility = () => (document.hidden ? stop() : start())
    document.addEventListener('visibilitychange', onVisibility)
    start()
    return () => {
      stop()
      observer.disconnect()
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [])

  return (
    <svg
      ref={svgRef}
      viewBox={`0 0 ${W} ${H}`}
      preserveAspectRatio="xMidYMid slice"
      className="block h-full w-full select-none"
      role="img"
      aria-label="Vulnerable code, packages, secrets, Kubernetes settings and CI steps flow in along red branching pipes, pass through GuardPipe, and leave along blue branches as their fixed versions."
    >
      <defs>
        {IN_LANES.map((l, i) => (
          <path key={i} id={`gp-in-${i}`} d={l.d} />
        ))}
        {OUT_LANES.map((l, i) => (
          <path key={i} id={`gp-out-${i}`} d={l.d} />
        ))}
        {/* Inflow fades in from the left edge and is absorbed at the pill;
            outflow emerges from the pill and fades out at the right edge. */}
        <linearGradient id="gp-in-fade" x1="0" x2="1">
          <stop offset="0%" stopColor="#fff" stopOpacity="0" />
          <stop offset="8%" stopColor="#fff" stopOpacity="1" />
          <stop offset="90%" stopColor="#fff" stopOpacity="1" />
          <stop offset="100%" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
        <linearGradient id="gp-out-fade" x1="0" x2="1">
          <stop offset="0%" stopColor="#fff" stopOpacity="0" />
          <stop offset="10%" stopColor="#fff" stopOpacity="1" />
          <stop offset="92%" stopColor="#fff" stopOpacity="1" />
          <stop offset="100%" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
        <mask id="gp-in-mask" maskUnits="userSpaceOnUse" x="0" y="0" width={W} height={H}>
          <rect x="0" y="0" width={IN_END + 10} height={H} fill="url(#gp-in-fade)" />
        </mask>
        <mask id="gp-out-mask" maskUnits="userSpaceOnUse" x="0" y="0" width={W} height={H}>
          <rect
            x={OUT_START - 10}
            y="0"
            width={W - OUT_START + 10}
            height={H}
            fill="url(#gp-out-fade)"
          />
        </mask>
        <filter id="gp-glow" x="-20%" y="-50%" width="140%" height="200%">
          <feGaussianBlur stdDeviation="10" />
        </filter>
      </defs>

      <Lanes
        lanes={IN_LANES}
        prefix="gp-in"
        stroke={RED_STROKE}
        commits={IN_COMMITS}
        commitStroke="rgba(248,113,113,0.6)"
        mask="gp-in-mask"
        textRefs={inTextRefs}
      />
      <Lanes
        lanes={OUT_LANES}
        prefix="gp-out"
        stroke={BLUE_STROKE}
        commits={OUT_COMMITS}
        commitStroke="rgba(143,227,255,0.75)"
        mask="gp-out-mask"
        textRefs={outTextRefs}
      />

      {/* ── the middleman: GuardPipe ── */}
      <circle
        ref={pulseRef}
        cx={NODE_X}
        cy={NODE_Y}
        r={40}
        fill="none"
        stroke="#8fe3ff"
        strokeWidth={1.5}
        style={{ opacity: 0 }}
      />
      <rect
        x={NODE_X - 98}
        y={NODE_Y - 38}
        width={196}
        height={76}
        rx={38}
        fill="#3b5bff"
        opacity={0.35}
        filter="url(#gp-glow)"
      />
      <rect
        x={NODE_X - 92}
        y={NODE_Y - 34}
        width={184}
        height={68}
        rx={34}
        fill="#ffffff"
        stroke="#0b1330"
        strokeWidth={2.5}
      />
      <image href={logoSrc} x={NODE_X - 76} y={NODE_Y - 17} width={34} height={34} />
      {Array.from({ length: BARS }, (_, k) => (
        <rect
          key={k}
          ref={(el) => {
            barRefs.current[k] = el
          }}
          x={NODE_X - 30 + k * 8}
          y={NODE_Y - 8}
          width={3}
          height={16}
          rx={1.5}
          fill="#0b1330"
        />
      ))}

      {/* ── fix chip ── */}
      <g ref={chipRef} style={{ opacity: 0 }}>
        <g transform={`translate(0 ${NODE_Y - 82})`}>
          <rect
            ref={chipBgRef}
            x={NODE_X - 80}
            y={-17}
            width={160}
            height={34}
            rx={17}
            fill="#065f46"
          />
          <path
            ref={checkRef}
            d="M-5 0 l4 4 l7 -8"
            fill="none"
            stroke="#ffffff"
            strokeWidth={2.2}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
          <text
            ref={chipTextRef}
            x={NODE_X - 50}
            y={5}
            fontSize={14}
            fontWeight={600}
            fill="#ffffff"
            fontFamily="Inter, ui-sans-serif, system-ui, sans-serif"
          >
            Secret removed
          </text>
        </g>
      </g>
    </svg>
  )
}
