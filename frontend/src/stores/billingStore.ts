import { create } from 'zustand'
import { ApiError } from '../lib/apiClient'
import { getBillingSummary, type BillingSummary } from '../lib/billingApi'

/**
 * The org's token balance, shared by every component that shows it (the
 * top-bar TokenBar, the scan launcher, the Billing page) so they all move
 * together. `notifyBillingChanged()` (lib/billingEvents.ts) re-fetches it
 * after anything that spends or adds tokens.
 */
interface BillingState {
  summary: BillingSummary | null
  /** true once we know billing isn't available here (billing off, or a
   * shared-project session) — the token UI then hides itself. */
  unavailable: boolean
  /** Change in balance since the previous fetch, with an id so the UI can
   * animate each change once. */
  lastChange: { id: number; delta: number } | null
  refresh: () => Promise<void>
  reset: () => void
}

let changeSeq = 0

export const useBillingStore = create<BillingState>((set, get) => ({
  summary: null,
  unavailable: false,
  lastChange: null,
  refresh: async () => {
    try {
      const next = await getBillingSummary()
      const prev = get().summary
      const delta = prev ? next.balance - prev.balance : 0
      set({
        summary: next,
        unavailable: false,
        lastChange: delta !== 0 ? { id: ++changeSeq, delta } : get().lastChange,
      })
    } catch (err) {
      if (err instanceof ApiError && (err.problem.status === 404 || err.problem.status === 403)) {
        set({ unavailable: true, summary: null })
      }
    }
  },
  reset: () => set({ summary: null, unavailable: false, lastChange: null }),
}))
