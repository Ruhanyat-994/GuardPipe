import { create } from 'zustand'
import { getProgress, getScan, listActiveScans, type OrgScanSummary } from '../lib/scansApi'
import { useAuthStore } from './authStore'
import { useToastStore } from './toastStore'

// Poll fast while something is in flight (the indicator shows live
// progress), slowly otherwise — slow enough to cost next to nothing, fast
// enough that a scan started elsewhere (a webhook push, a schedule, a
// teammate) shows up without a page refresh.
const ACTIVE_POLL_MS = 5_000
const IDLE_POLL_MS = 20_000
// Per-scan progress is one extra request each; the indicator only needs the
// first few to be meaningful.
const MAX_PROGRESS_LOOKUPS = 5

interface ActiveScansState {
  scans: OrgScanSummary[]
  // scan id → overall progress_pct, for running scans only.
  progress: Record<string, number>
  loaded: boolean
  start: () => void
  stop: () => void
  /** Poll right now — called after launching a scan so the indicator
   * picks it up immediately instead of on the next tick. */
  refresh: () => void
}

let timer: number | null = null
let running = false
// Scan ids seen in flight on the previous tick, and the org they belong
// to. null = no baseline yet: the first tick after start (or after an org
// switch) must not announce anything, only record what's already running.
let known: Set<string> | null = null
let knownOrg: string | null = null

function clearTimer() {
  if (timer !== null) {
    window.clearTimeout(timer)
    timer = null
  }
}

/** Tells the user a scan they're not looking at has finished. Looks the
 * scan up once so the toast can carry the final status and score. */
async function announceFinished(scanId: string, summary: OrgScanSummary | undefined) {
  // They're already watching it — ScanDetailPage shows the result itself.
  if (window.location.pathname === `/scans/${scanId}`) return
  let scan
  try {
    scan = await getScan(scanId)
  } catch {
    return // gone, or no longer visible to this session — nothing to say
  }
  const label = `${summary?.project_name ?? 'A project'} · Scan #${scan.scan_number}`
  const href = `/scans/${scanId}`
  const push = useToastStore.getState().push
  if (scan.status === 'completed') {
    const risk = scan.risk
    push({
      tone: risk?.verdict === 'block' ? 'danger' : risk?.verdict === 'warn' ? 'warning' : 'success',
      title: 'Scan finished',
      body: risk ? `${label} — risk score ${risk.score} (${risk.verdict})` : label,
      href,
      hrefLabel: 'View results',
    })
  } else if (scan.status === 'failed') {
    push({ tone: 'danger', title: 'Scan failed', body: label, href, hrefLabel: 'View details' })
  } else if (scan.status === 'cancelled') {
    push({ tone: 'info', title: 'Scan cancelled', body: label, href, hrefLabel: 'View' })
  }
}

/** The running-scans indicator's state, and the one poller behind it —
 * started by AppShell so it runs on every authenticated page, which is what
 * lets a user leave a scan's page and still see it progress and be told
 * when it's done. Scans themselves already run server-side regardless of
 * what page is open; this only watches them. */
export const useActiveScansStore = create<ActiveScansState>((set, get) => {
  async function tick() {
    clearTimer()
    const orgId = useAuthStore.getState().user?.orgId ?? null
    if (orgId !== knownOrg) {
      known = null
      knownOrg = orgId
    }
    try {
      const { data } = await listActiveScans()
      const progressPairs = await Promise.all(
        data
          .filter((s) => s.status === 'running')
          .slice(0, MAX_PROGRESS_LOOKUPS)
          .map((s) =>
            getProgress(s.id)
              .then((p) => [s.id, p.progress_pct] as const)
              .catch(() => null),
          ),
      )
      if (!running) return

      const ids = new Set(data.map((s) => s.id))
      if (known !== null && knownOrg === orgId) {
        const previous = get().scans
        for (const id of known) {
          if (!ids.has(id))
            void announceFinished(
              id,
              previous.find((s) => s.id === id),
            )
        }
      }
      known = ids

      const progress: Record<string, number> = {}
      for (const pair of progressPairs) if (pair) progress[pair[0]] = pair[1]
      set({ scans: data, progress, loaded: true })
    } catch {
      // A failed background poll keeps whatever it last knew, same as
      // NotificationPanel — the next tick tries again.
    } finally {
      if (running) {
        // A refresh() can overlap a scheduled tick — keep exactly one timer.
        clearTimer()
        timer = window.setTimeout(
          () => void tick(),
          get().scans.length > 0 ? ACTIVE_POLL_MS : IDLE_POLL_MS,
        )
      }
    }
  }

  return {
    scans: [],
    progress: {},
    loaded: false,
    start: () => {
      if (running) return
      running = true
      void tick()
    },
    stop: () => {
      running = false
      clearTimer()
      known = null
      knownOrg = null
      set({ scans: [], progress: {}, loaded: false })
    },
    refresh: () => {
      if (running) void tick()
    },
  }
})
