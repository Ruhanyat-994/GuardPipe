import { type ReactNode, useState } from 'react'
import {
  AlertTriangle,
  Check,
  Copy,
  Crosshair,
  ExternalLink,
  FileCode2,
  Info,
  ShieldAlert,
  ShieldCheck,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { cn } from '../../lib/cn'
import { buildFindingBlobUrl } from '../../lib/repoLink'
import type { AssistResponse } from '../../lib/assistApi'
import type { Session } from '../../stores/assistantStore'

function useCopy() {
  const [copied, setCopied] = useState(false)
  return {
    copied,
    copy: (text: string) => {
      void navigator.clipboard?.writeText(text).then(() => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1400)
      })
    },
  }
}

function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const { copied, copy } = useCopy()
  return (
    <button
      type="button"
      onClick={() => copy(text)}
      className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium text-white/60 transition-colors hover:bg-white/10 hover:text-white"
    >
      {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
      {copied ? 'Copied' : label}
    </button>
  )
}

function CodeBlock({ code, label }: { code: string; label?: string }) {
  return (
    <div className="overflow-hidden rounded-lg border border-white/10 bg-black/50">
      <div className="flex items-center justify-between border-b border-white/5 px-3 py-1">
        <span className="font-mono text-[10.5px] text-white/35">{label ?? 'snippet'}</span>
        <CopyButton text={code} />
      </div>
      <pre className="max-h-64 overflow-auto px-3 py-2.5 font-mono text-[12px] leading-5 text-white/85">
        {code}
      </pre>
    </div>
  )
}

/** Unified diff with +/- colouring. */
function DiffView({ patch }: { patch: string }) {
  const lines = patch.replace(/\n$/, '').split('\n')
  return (
    <div className="overflow-hidden rounded-lg border border-white/10 bg-black/60">
      <div className="flex items-center justify-between border-b border-white/5 px-3 py-1">
        <span className="font-mono text-[10.5px] text-white/35">patch.diff · git apply</span>
        <CopyButton text={patch} label="Copy patch" />
      </div>
      <pre className="max-h-80 overflow-auto py-2 font-mono text-[12px] leading-5">
        {lines.map((l, i) => {
          const tone =
            l.startsWith('+++') || l.startsWith('---')
              ? 'text-white/45'
              : l.startsWith('@@')
                ? 'text-sky-300/80 bg-sky-400/5'
                : l.startsWith('+')
                  ? 'text-emerald-300 bg-emerald-400/10'
                  : l.startsWith('-')
                    ? 'text-rose-300 bg-rose-400/10'
                    : 'text-white/70'
          return (
            <div key={i} className={cn('px-3 whitespace-pre', tone)}>
              {l || ' '}
            </div>
          )
        })}
      </pre>
    </div>
  )
}

const CONFIDENCE: Record<string, string> = {
  high: 'bg-emerald-400/15 text-emerald-300',
  medium: 'bg-amber-400/15 text-amber-300',
  low: 'bg-rose-400/15 text-rose-300',
}

export function Confidence({ level }: { level: string }) {
  return (
    <span className={cn('rounded-full px-2 py-0.5 text-[10.5px] font-semibold', CONFIDENCE[level])}>
      {level} confidence
    </span>
  )
}

function Section({
  icon: Icon,
  title,
  children,
}: {
  icon: typeof Info
  title: string
  children: ReactNode
}) {
  return (
    <div className="flex gap-2.5">
      <Icon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-white/40" aria-hidden="true" />
      <div className="min-w-0">
        <p className="text-[11px] font-semibold tracking-wide text-white/45 uppercase">{title}</p>
        <p className="mt-0.5 text-[13px] leading-5 text-white/85">{children}</p>
      </div>
    </div>
  )
}

export function AnswerBody({ data, session }: { data: AssistResponse; session: Session }) {
  if (data.explain) {
    const e = data.explain
    return (
      <div className="flex flex-col gap-3">
        <Section icon={Info} title="What it is">
          {e.what}
        </Section>
        <Section icon={ShieldAlert} title="Why it matters">
          {e.why_it_matters}
        </Section>
        <Section icon={Crosshair} title="How it could be exploited">
          {e.how_exploited}
        </Section>
      </div>
    )
  }
  if (data.remediate) {
    const r = data.remediate
    return (
      <div className="flex flex-col gap-3">
        <p className="text-[13px] leading-5 text-white/85">{r.summary}</p>
        <ol className="flex flex-col gap-3">
          {r.steps.map((st, i) => (
            <li key={i} className="flex gap-2.5">
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-indigo-400/20 text-[11px] font-semibold text-indigo-200">
                {i + 1}
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-semibold text-white">{st.title}</p>
                <p className="mt-0.5 text-[13px] leading-5 text-white/75">{st.detail}</p>
                {st.code && (
                  <div className="mt-2">
                    <CodeBlock code={st.code} />
                  </div>
                )}
              </div>
            </li>
          ))}
        </ol>
        <div className="flex gap-2 rounded-lg bg-emerald-400/5 px-3 py-2 ring-1 ring-emerald-400/15">
          <ShieldCheck
            className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-300"
            aria-hidden="true"
          />
          <p className="text-[12.5px] leading-5 text-white/80">
            <span className="font-semibold text-emerald-200">Verify: </span>
            {r.verification}
          </p>
        </div>
      </div>
    )
  }
  if (data.fix) {
    const f = data.fix
    const path = session.finding.location.path ?? session.finding.location.file
    return (
      <div className="flex flex-col gap-3">
        <p className="text-[13px] leading-5 text-white/85">{f.explanation}</p>
        <DiffView patch={f.patch} />
        {data.source_used && path && (
          <p className="flex items-start gap-1.5 text-[11.5px] text-white/45">
            <FileCode2 className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />
            <span>
              Written against the real <span className="font-mono text-white/65">{path}</span> at
              the scanned commit.
            </span>
          </p>
        )}
        {f.caveats.length > 0 && (
          <div className="flex flex-col gap-1.5 rounded-lg bg-amber-400/5 px-3 py-2 ring-1 ring-amber-400/15">
            {f.caveats.map((c, i) => (
              <p key={i} className="flex gap-2 text-[12.5px] leading-5 text-amber-100/85">
                <AlertTriangle
                  className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-300"
                  aria-hidden="true"
                />
                {c}
              </p>
            ))}
          </div>
        )}
        <p className="text-[11px] text-white/35">
          Review before applying — AI patches can be wrong. The scanner&rsquo;s own guidance stays
          in the finding&rsquo;s Remediation tab.
        </p>
      </div>
    )
  }
  return null
}

/** "Where is it?" — answered locally from the finding's location; free. */
export function LocateBody({ session }: { session: Session }) {
  const f = session.finding
  const loc = f.location
  const url = buildFindingBlobUrl(loc, session.repository, session.gitRef)
  const rows: [string, string][] = []
  if (loc.path) rows.push(['File', loc.line_start ? `${loc.path}:${loc.line_start}` : loc.path])
  if (loc.file) rows.push(['Manifest', loc.line_start ? `${loc.file}:${loc.line_start}` : loc.file])
  if (loc.package) rows.push(['Package', `${loc.package}${loc.version ? `@${loc.version}` : ''}`])
  if (loc.image) rows.push(['Image', loc.image])
  if (loc.url) rows.push(['Endpoint', loc.url])
  else if (loc.host) rows.push(['Host', `${loc.host}${loc.port ? `:${loc.port}` : ''}`])
  rows.push(['Rule', f.rule_id])

  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-[12.5px]">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-white/45">{k}</dt>
            <dd className="min-w-0 font-mono break-all text-white/85">{v}</dd>
          </div>
        ))}
      </dl>
      {f.evidence[0]?.value && <CodeBlock code={f.evidence[0].value} label="evidence" />}
      {url ? (
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex w-fit items-center gap-1.5 rounded-full bg-white px-3.5 py-1.5 text-[12.5px] font-semibold text-neutral-950 hover:bg-white/90"
        >
          Open on GitHub
          <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
        </a>
      ) : (
        <p className="text-[12px] text-white/45">
          {loc.type === 'dependency' || loc.type === 'image'
            ? 'This lives in a package or image layer, not at a line of your repository — nothing to link to.'
            : 'No GitHub link is available for this finding.'}
        </p>
      )}
    </div>
  )
}

export function ShortfallLink() {
  return (
    <Link
      to="/settings/billing"
      className="font-semibold text-indigo-300 underline-offset-2 hover:underline"
    >
      Top up tokens
    </Link>
  )
}
