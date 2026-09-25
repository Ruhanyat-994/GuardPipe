import { create } from 'zustand'
import { ApiError } from '../lib/apiClient'
import { notifyBillingChanged } from '../lib/billingEvents'
import { runAssist, type AssistAction, type AssistResponse } from '../lib/assistApi'
import type { FindingListItem } from '../lib/scansApi'
import type { Repository } from '../lib/projectsApi'

/** Every command the panel offers. `locate` is answered locally from the
 * finding's own location — free, no model call. */
export type Command = AssistAction | 'locate'

export type Message =
  | { id: number; kind: 'command'; command: Command }
  | { id: number; kind: 'answer'; command: AssistAction; data: AssistResponse }
  | { id: number; kind: 'locate' }
  | {
      id: number
      kind: 'error'
      command: Command
      text: string
      // Set for "not enough tokens", so the panel can link to billing.
      shortfall?: { required: number; available: number }
    }

export interface Session {
  finding: FindingListItem
  repository: Repository | null
  gitRef: string | null
  messages: Message[]
  pending: Command | null
}

// Messenger-style: a few findings can stay open as chat heads at once.
const MAX_SESSIONS = 4

interface AssistantState {
  sessions: Session[]
  activeId: string | null
  /** true = only the chat heads show; the panel is collapsed. */
  minimized: boolean
  open: (finding: FindingListItem, repository: Repository | null, gitRef: string | null) => void
  focus: (findingId: string) => void
  minimize: () => void
  close: (findingId: string) => void
  run: (findingId: string, command: Command) => Promise<void>
}

let nextId = 1

function update(sessions: Session[], findingId: string, fn: (s: Session) => Session): Session[] {
  return sessions.map((s) => (s.finding.id === findingId ? fn(s) : s))
}

export const useAssistantStore = create<AssistantState>((set, get) => ({
  sessions: [],
  activeId: null,
  minimized: false,

  open: (finding, repository, gitRef) => {
    const existing = get().sessions.find((s) => s.finding.id === finding.id)
    if (existing) {
      set({ activeId: finding.id, minimized: false })
      return
    }
    const fresh: Session = { finding, repository, gitRef, messages: [], pending: null }
    // Oldest head drops off when the tray is full.
    set((st) => ({
      sessions: [...st.sessions, fresh].slice(-MAX_SESSIONS),
      activeId: finding.id,
      minimized: false,
    }))
  },

  focus: (findingId) => set({ activeId: findingId, minimized: false }),
  minimize: () => set({ minimized: true }),

  close: (findingId) =>
    set((st) => {
      const sessions = st.sessions.filter((s) => s.finding.id !== findingId)
      const activeId =
        st.activeId === findingId ? (sessions.at(-1)?.finding.id ?? null) : st.activeId
      return { sessions, activeId, minimized: sessions.length === 0 ? false : st.minimized }
    }),

  run: async (findingId, command) => {
    const session = get().sessions.find((s) => s.finding.id === findingId)
    if (!session || session.pending) return

    const ask: Message = { id: nextId++, kind: 'command', command }
    if (command === 'locate') {
      set((st) => ({
        sessions: update(st.sessions, findingId, (s) => ({
          ...s,
          messages: [...s.messages, ask, { id: nextId++, kind: 'locate' }],
        })),
      }))
      return
    }

    // Asked before in this session: show the answer again, no request.
    const previous = session.messages.find(
      (m): m is Extract<Message, { kind: 'answer' }> =>
        m.kind === 'answer' && m.command === command,
    )
    set((st) => ({
      sessions: update(st.sessions, findingId, (s) => ({
        ...s,
        messages: [...s.messages, ask],
        pending: previous ? null : command,
      })),
    }))
    if (previous) {
      set((st) => ({
        sessions: update(st.sessions, findingId, (s) => ({
          ...s,
          messages: [
            ...s.messages,
            {
              ...previous,
              id: nextId++,
              data: { ...previous.data, tokens_charged: 0, already_paid: true },
            },
          ],
        })),
      }))
      return
    }

    let reply: Message
    try {
      const data = await runAssist(findingId, command)
      reply = { id: nextId++, kind: 'answer', command, data }
      if (data.tokens_charged > 0) notifyBillingChanged()
    } catch (err) {
      const problem = err instanceof ApiError ? err.problem : null
      reply = {
        id: nextId++,
        kind: 'error',
        command,
        text: problem?.detail ?? 'Something went wrong. Nothing was charged.',
        shortfall:
          problem?.code === 'billing.insufficient_tokens' && problem.required !== undefined
            ? { required: problem.required, available: problem.available ?? 0 }
            : undefined,
      }
    }
    set((st) => ({
      sessions: update(st.sessions, findingId, (s) => ({
        ...s,
        messages: [...s.messages, reply],
        pending: null,
      })),
    }))
  },
}))
