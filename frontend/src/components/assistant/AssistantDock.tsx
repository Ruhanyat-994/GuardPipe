import { useEffect, useRef, useState } from 'react'
import { BookOpen, ListChecks, MapPin, Minus, Sparkles, Wand2, X } from 'lucide-react'
import { cn } from '../../lib/cn'
import { getCatalog } from '../../lib/billingApi'
import { useAuthStore } from '../../stores/authStore'
import { useAssistantStore, type Command, type Session } from '../../stores/assistantStore'
import { AnswerBody, Confidence, LocateBody, ShortfallLink } from './AssistantAnswer'

/**
 * The finding assistant — a docked, messenger-style panel on the right of
 * every authenticated page, with "chat heads" for each open finding. It
 * never navigates away and has no free-text input: the user picks a
 * command, the answer lands as a card. Commands that call the model show
 * their token cost up front (prices come from the billing catalogue).
 *
 * Deliberately always dark (its own palette, not the app's light/dark
 * tokens) — it's an overlay with its own identity, like the reference.
 */

const SEVERITY: Record<string, string> = {
  critical: 'var(--sev-critical)',
  high: 'var(--sev-high)',
  medium: 'var(--sev-medium)',
  low: 'var(--sev-low)',
  informational: 'var(--sev-info)',
}

const COMMANDS: {
  id: Command
  label: string
  blurb: string
  icon: typeof BookOpen
  tag: string
  glow: string
}[] = [
  {
    id: 'explain',
    label: 'Define it',
    blurb: 'What it is and why it matters',
    icon: BookOpen,
    tag: 'bg-sky-300/90 text-sky-950',
    glow: 'from-sky-500/10',
  },
  {
    id: 'locate',
    label: 'Where is it?',
    blurb: 'Exact file, line and GitHub link',
    icon: MapPin,
    tag: 'bg-emerald-300/90 text-emerald-950',
    glow: 'from-emerald-500/10',
  },
  {
    id: 'remediate',
    label: 'Remediation',
    blurb: 'A step-by-step plan for your code',
    icon: ListChecks,
    tag: 'bg-rose-300/90 text-rose-950',
    glow: 'from-rose-500/10',
  },
  {
    id: 'fix',
    label: 'Fix it',
    blurb: 'A ready-to-apply patch',
    icon: Wand2,
    tag: 'bg-violet-300/90 text-violet-950',
    glow: 'from-violet-500/10',
  },
]

const LABEL: Record<Command, string> = {
  explain: 'Define it',
  locate: 'Where is it?',
  remediate: 'Remediation',
  fix: 'Fix it',
}

let pricesCache: Record<string, number> | null | undefined

/** AI command prices from the billing catalogue — fetched once. null =
 * billing is off (everything's free) or the catalogue is unavailable. */
function usePrices() {
  const [prices, setPrices] = useState<Record<string, number> | null | undefined>(pricesCache)
  useEffect(() => {
    if (pricesCache !== undefined) return
    getCatalog()
      .then((c) => {
        pricesCache = c.ai_prices ?? null
        setPrices(pricesCache)
      })
      .catch(() => {
        pricesCache = null
        setPrices(null)
      })
  }, [])
  return prices
}

function costLabel(cmd: Command, prices: Record<string, number> | null | undefined) {
  if (cmd === 'locate') return 'Free'
  const p = prices?.[cmd]
  return p ? `${p.toLocaleString()} tokens` : ''
}

/** Fix it needs a file line to patch (code, config, workflow, manifest). */
function patchable(session: Session) {
  const t = session.finding.location.type
  return t === 'file' || t === 'k8s'
}

const NOT_PATCHABLE =
  'Nothing to patch in your code for this kind of finding — use Remediation for the upgrade steps.'

export function AssistantDock() {
  const sessions = useAssistantStore((s) => s.sessions)
  const activeId = useAssistantStore((s) => s.activeId)
  const minimized = useAssistantStore((s) => s.minimized)
  const active = sessions.find((s) => s.finding.id === activeId) ?? null

  if (sessions.length === 0) return null

  return (
    <>
      {active && !minimized && <Panel session={active} />}
      <ChatHeads sessions={sessions} activeId={minimized ? null : activeId} />
    </>
  )
}

function ChatHeads({ sessions, activeId }: { sessions: Session[]; activeId: string | null }) {
  const focus = useAssistantStore((s) => s.focus)
  const close = useAssistantStore((s) => s.close)
  return (
    <div className="fixed right-5 bottom-5 z-[55] flex flex-col-reverse items-end gap-2.5">
      {sessions.map((s) => (
        <div key={s.finding.id} className="group relative">
          <button
            type="button"
            onClick={() => focus(s.finding.id)}
            title={s.finding.title}
            aria-label={`Open AI assistant for ${s.finding.title}`}
            className={cn(
              'gp-head relative flex h-12 w-12 items-center justify-center rounded-full bg-[#0b0c12] shadow-[0_10px_30px_rgba(0,0,0,0.45)] ring-2 transition-transform hover:scale-105',
              activeId === s.finding.id ? 'ring-indigo-300' : 'ring-white/15',
            )}
            style={{
              boxShadow: `0 0 0 3px ${SEVERITY[s.finding.severity]}33, 0 10px 30px rgba(0,0,0,0.45)`,
            }}
          >
            <Sparkles className="h-5 w-5 text-indigo-200" aria-hidden="true" />
            <span
              className="absolute -top-0.5 -right-0.5 h-3.5 w-3.5 rounded-full ring-2 ring-[#0b0c12]"
              style={{ backgroundColor: SEVERITY[s.finding.severity] }}
            />
            {s.pending && (
              <span className="absolute inset-0 animate-ping rounded-full ring-2 ring-indigo-400/50" />
            )}
          </button>
          <button
            type="button"
            onClick={() => close(s.finding.id)}
            aria-label="Close this conversation"
            className="absolute -top-1.5 -left-1.5 hidden h-5 w-5 items-center justify-center rounded-full bg-neutral-800 text-white/80 ring-1 ring-white/20 group-hover:flex hover:bg-neutral-700"
          >
            <X className="h-3 w-3" />
          </button>
        </div>
      ))}
    </div>
  )
}

function Panel({ session }: { session: Session }) {
  const minimize = useAssistantStore((s) => s.minimize)
  const close = useAssistantStore((s) => s.close)
  const run = useAssistantStore((s) => s.run)
  const displayName = useAuthStore((s) => s.user?.displayName ?? '')
  const prices = usePrices()
  const scrollRef = useRef<HTMLDivElement>(null)
  const f = session.finding
  const firstName = displayName.split(' ')[0]
  const started = session.messages.length > 0

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [session.messages.length, session.pending])

  return (
    <aside
      className="gp-assistant fixed top-16 right-20 bottom-5 z-[55] flex w-[min(440px,calc(100vw-6rem))] flex-col overflow-hidden rounded-2xl border border-white/10 bg-[#07080c] text-white shadow-[0_30px_80px_rgba(0,0,0,0.6)]"
      aria-label="GuardPipe AI assistant"
    >
      {/* backdrop: faint grid + a soft aurora glow, like the reference */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 opacity-[0.07]"
        style={{
          backgroundImage:
            'linear-gradient(rgba(255,255,255,0.6) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.6) 1px, transparent 1px)',
          backgroundSize: '28px 28px',
        }}
      />
      <div
        aria-hidden="true"
        className="gp-orb pointer-events-none absolute -top-24 left-1/2 h-64 w-72 -translate-x-1/2 rounded-full blur-3xl"
        style={{
          background:
            'radial-gradient(closest-side, rgba(129,140,248,0.45), rgba(139,92,246,0.25) 55%, transparent)',
        }}
      />

      {/* header */}
      <header className="relative flex items-start gap-3 border-b border-white/5 px-4 py-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-indigo-400 to-violet-500">
          <Sparkles className="h-4 w-4 text-white" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-[11px] font-semibold tracking-wide text-white/45 uppercase">
            GuardPipe AI
          </p>
          <p className="line-clamp-2 text-[13.5px] leading-5 font-semibold text-white">{f.title}</p>
          <div className="mt-1 flex items-center gap-2">
            <span
              className="rounded-full px-1.5 py-px text-[10px] font-bold text-white uppercase"
              style={{ backgroundColor: SEVERITY[f.severity] }}
            >
              {f.severity}
            </span>
            <span className="truncate font-mono text-[10.5px] text-white/40">{f.rule_id}</span>
          </div>
        </div>
        <button
          type="button"
          onClick={minimize}
          aria-label="Minimise"
          className="rounded-md p-1.5 text-white/50 hover:bg-white/10 hover:text-white"
        >
          <Minus className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={() => close(f.id)}
          aria-label="Close"
          className="rounded-md p-1.5 text-white/50 hover:bg-white/10 hover:text-white"
        >
          <X className="h-4 w-4" />
        </button>
      </header>

      {/* conversation */}
      <div ref={scrollRef} className="relative flex-1 overflow-y-auto px-4 py-5">
        {!started ? (
          <div className="gp-rise">
            <p className="text-[26px] leading-tight font-light tracking-tight text-white">
              Hey{firstName ? ` ${firstName}` : ''}!
            </p>
            <p className="text-[26px] leading-tight font-light tracking-tight text-white/55">
              What do you want to know?
            </p>
            <div className="mt-6 grid grid-cols-2 gap-2.5">
              {COMMANDS.map((c, i) => (
                <button
                  key={c.id}
                  type="button"
                  disabled={c.id === 'fix' && !patchable(session)}
                  title={c.id === 'fix' && !patchable(session) ? NOT_PATCHABLE : undefined}
                  onClick={() => void run(f.id, c.id)}
                  className={cn(
                    'gp-rise group flex flex-col items-start gap-2 rounded-xl border border-white/10 bg-gradient-to-br to-transparent p-3 text-left transition-colors hover:border-white/25 disabled:cursor-not-allowed disabled:opacity-40',
                    c.glow,
                  )}
                  style={{ animationDelay: `${80 + i * 60}ms` }}
                >
                  <span className={cn('rounded-md px-2 py-0.5 text-[12px] font-semibold', c.tag)}>
                    {c.label}
                  </span>
                  <span className="text-[12px] leading-4 text-white/55">{c.blurb}</span>
                  <span className="mt-auto text-[10.5px] font-medium text-white/35">
                    {costLabel(c.id, prices)}
                  </span>
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            {session.messages.map((m) => {
              if (m.kind === 'command') {
                return (
                  <div key={m.id} className="gp-rise flex justify-end">
                    <span className="rounded-2xl rounded-br-md bg-indigo-500/25 px-3 py-1.5 text-[12.5px] font-medium text-indigo-100 ring-1 ring-indigo-400/30">
                      {LABEL[m.command]}
                    </span>
                  </div>
                )
              }
              return (
                <div
                  key={m.id}
                  className={cn(
                    'gp-rise rounded-2xl rounded-tl-md border p-3.5',
                    m.kind === 'error'
                      ? 'border-rose-400/25 bg-rose-500/5'
                      : 'border-white/10 bg-white/[0.035] backdrop-blur',
                  )}
                >
                  <div className="mb-2.5 flex flex-wrap items-center gap-2">
                    <Sparkles className="h-3.5 w-3.5 text-indigo-300" aria-hidden="true" />
                    <span className="text-[11.5px] font-semibold text-white/70">
                      {m.kind === 'locate' ? 'Where is it?' : LABEL[m.command]}
                    </span>
                    {m.kind === 'answer' && m.data.explain && (
                      <Confidence level={m.data.explain.confidence} />
                    )}
                    {m.kind === 'answer' && m.data.remediate && (
                      <Confidence level={m.data.remediate.confidence} />
                    )}
                    {m.kind === 'answer' && m.data.fix && (
                      <Confidence level={m.data.fix.confidence} />
                    )}
                    <span className="ml-auto text-[10.5px] text-white/35">
                      {m.kind === 'answer'
                        ? m.data.tokens_charged > 0
                          ? `−${m.data.tokens_charged.toLocaleString()} tokens`
                          : m.data.already_paid
                            ? 'already paid'
                            : ''
                        : m.kind === 'locate'
                          ? 'free'
                          : ''}
                    </span>
                  </div>
                  {m.kind === 'answer' && <AnswerBody data={m.data} session={session} />}
                  {m.kind === 'locate' && <LocateBody session={session} />}
                  {m.kind === 'error' && (
                    <p className="text-[13px] leading-5 text-rose-100/90">
                      {m.shortfall
                        ? `This needs ${m.shortfall.required.toLocaleString()} tokens and you have ${m.shortfall.available.toLocaleString()}. `
                        : `${m.text} `}
                      {m.shortfall && <ShortfallLink />}
                    </p>
                  )}
                </div>
              )
            })}
            {session.pending && (
              <div className="gp-rise flex items-center gap-2.5 rounded-2xl rounded-tl-md border border-white/10 bg-white/[0.035] px-3.5 py-3">
                <Sparkles
                  className="h-3.5 w-3.5 animate-pulse text-indigo-300"
                  aria-hidden="true"
                />
                <span className="gp-shimmer text-[12.5px] font-medium">
                  {session.pending === 'fix'
                    ? 'Reading the file and writing a patch…'
                    : session.pending === 'remediate'
                      ? 'Working out the steps…'
                      : 'Thinking…'}
                </span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* command box — the reference's input box, with commands instead of typing */}
      <footer className="relative border-t border-white/5 p-3">
        <div className="rounded-xl border border-white/10 bg-white/[0.04] p-2.5">
          <p className="mb-2 flex items-center gap-1.5 px-0.5 text-[11.5px] text-white/40">
            <Sparkles className="h-3 w-3" aria-hidden="true" />
            {started
              ? 'Ask something else about this finding'
              : 'Pick a command — no typing needed'}
          </p>
          <div className="flex flex-wrap gap-1.5">
            {COMMANDS.map((c) => (
              <button
                key={c.id}
                type="button"
                disabled={session.pending !== null || (c.id === 'fix' && !patchable(session))}
                title={c.id === 'fix' && !patchable(session) ? NOT_PATCHABLE : undefined}
                onClick={() => void run(f.id, c.id)}
                className="inline-flex items-center gap-1.5 rounded-full bg-white/[0.06] px-2.5 py-1 text-[12px] font-medium text-white/80 ring-1 ring-white/10 transition-colors hover:bg-white/[0.12] hover:text-white disabled:opacity-40"
              >
                <c.icon className="h-3 w-3" aria-hidden="true" />
                {c.label}
                {costLabel(c.id, prices) && (
                  <span className="text-[10px] text-white/35">· {costLabel(c.id, prices)}</span>
                )}
              </button>
            ))}
          </div>
        </div>
      </footer>
    </aside>
  )
}
