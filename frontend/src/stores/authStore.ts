import { create } from 'zustand'
import { setAccessTokenGetter } from '../lib/apiClient'

/**
 * Client state for auth only — everything else is server state via
 * TanStack Query, added when Phase 2 needs it
 * (documentation/08-frontend-architecture.md §1, "Zustand — auth/UI only").
 *
 * This is a placeholder: `login`/`logout` don't call the API yet (there is
 * no identity module to call — that's Phase 2). The shape here is what
 * Phase 2's real login mutation will set via `setSession`.
 */

export interface AuthUser {
  id: string
  email: string
  displayName: string
  role: 'admin' | 'member' | 'viewer'
}

interface AuthState {
  user: AuthUser | null
  accessToken: string | null
  isAuthenticated: boolean
  setSession: (user: AuthUser, accessToken: string) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  accessToken: null,
  isAuthenticated: false,
  setSession: (user, accessToken) => set({ user, accessToken, isAuthenticated: true }),
  logout: () => set({ user: null, accessToken: null, isAuthenticated: false }),
}))

// Wires apiClient's Authorization header to whatever token is currently in
// the store, without apiClient importing this store directly (see
// apiClient.ts's comment on why the dependency points this direction).
setAccessTokenGetter(() => useAuthStore.getState().accessToken)
