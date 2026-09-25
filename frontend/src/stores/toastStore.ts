import { create } from 'zustand'

export type ToastTone = 'success' | 'warning' | 'danger' | 'info'

export interface Toast {
  id: number
  tone: ToastTone
  title: string
  body?: string
  // Optional in-app link rendered as the toast's action ("View report").
  href?: string
  hrefLabel?: string
}

interface ToastState {
  toasts: Toast[]
  push: (toast: Omit<Toast, 'id'>) => void
  dismiss: (id: number) => void
}

// How long a toast stays before dismissing itself. Long enough to read a
// scan result and click through, short enough not to pile up.
const TOAST_TTL_MS = 12_000
const MAX_TOASTS = 4

let nextId = 1

/** App-wide transient notices, rendered by `Toaster` inside AppShell. The
 * first producer is the running-scans poller (activeScansStore) telling the
 * user a scan they walked away from has finished. */
export const useToastStore = create<ToastState>((set, get) => ({
  toasts: [],
  push: (toast) => {
    const id = nextId++
    set((s) => ({ toasts: [...s.toasts, { ...toast, id }].slice(-MAX_TOASTS) }))
    window.setTimeout(() => get().dismiss(id), TOAST_TTL_MS)
  },
  dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}))
