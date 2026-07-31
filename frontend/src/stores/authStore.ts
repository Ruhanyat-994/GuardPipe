import { create } from 'zustand'
import { apiClient, setAccessTokenGetter, setTokenExpiredHandler } from '../lib/apiClient'

/**
 * Client state for auth only — everything else is server state, added via
 * TanStack Query when a later phase needs it
 * (documentation/08-frontend-architecture.md §1, "Zustand — auth/UI only").
 *
 * The access token lives here, in memory, only — never localStorage
 * (documentation/07-api-specification.md §2: "access token held in memory
 * only — never localStorage (XSS exfiltration)"). A page reload always
 * loses it, which is fine: bootstrap() below silently refreshes it back
 * from the httpOnly refresh cookie.
 */

export interface AuthUser {
  id: string
  email: string
  displayName: string
  role: 'admin' | 'member' | 'viewer'
}

interface UserResponse {
  id: string
  email: string
  display_name: string
  role: AuthUser['role']
}

interface LoginResponse {
  access_token: string
  token_type: string
  expires_in: number
  user: UserResponse
}

interface RefreshResponse {
  access_token: string
  token_type: string
  expires_in: number
}

function fromUserResponse(u: UserResponse): AuthUser {
  return { id: u.id, email: u.email, displayName: u.display_name, role: u.role }
}

interface AuthState {
  user: AuthUser | null
  accessToken: string | null
  isAuthenticated: boolean
  /** True until the initial silent-refresh attempt (bootstrap) resolves —
   * lets the router avoid a login-page flash on a hard reload while a
   * session might still be valid. */
  isInitializing: boolean

  login: (email: string, password: string) => Promise<void>
  register: (email: string, displayName: string, password: string) => Promise<void>
  logout: () => Promise<void>
  bootstrap: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  accessToken: null,
  isAuthenticated: false,
  isInitializing: true,

  login: async (email, password) => {
    const res = await apiClient.post<LoginResponse>('/auth/login', { email, password })
    set({ accessToken: res.access_token, user: fromUserResponse(res.user), isAuthenticated: true })
  },

  register: async (email, displayName, password) => {
    await apiClient.post('/auth/register', { email, display_name: displayName, password })
    // Register doesn't log the user in (documentation/07-api-specification.md
    // §2 — it returns the created user, not tokens); the caller navigates
    // to /login afterwards.
  },

  logout: async () => {
    try {
      await apiClient.post('/auth/logout')
    } finally {
      set({ user: null, accessToken: null, isAuthenticated: false })
    }
  },

  // Called once at app startup: attempts a silent refresh using the
  // httpOnly cookie, so a page reload doesn't force a re-login as long as
  // the refresh token is still valid.
  bootstrap: async () => {
    const token = await refreshAccessToken()
    if (!token) {
      set({ isInitializing: false })
      return
    }
    try {
      const me = await apiClient.get<UserResponse>('/auth/me')
      set({ user: fromUserResponse(me), isAuthenticated: true, isInitializing: false })
    } catch {
      set({ user: null, accessToken: null, isAuthenticated: false, isInitializing: false })
    }
  },
}))

// --- token wiring: connects apiClient to this store without apiClient
// importing it directly (see apiClient.ts's comment on the dependency
// direction) ---

setAccessTokenGetter(() => useAuthStore.getState().accessToken)

// Single-flight: concurrent 401s all await the same in-flight refresh
// instead of each firing their own (documentation/08-frontend-architecture.md
// §Phase 2, "Token storage + refresh with single-flight — never fire ten
// concurrent refreshes").
let refreshPromise: Promise<string | null> | null = null

async function refreshAccessToken(): Promise<string | null> {
  if (refreshPromise) {
    return refreshPromise
  }
  refreshPromise = (async () => {
    try {
      const res = await apiClient.post<RefreshResponse>('/auth/refresh')
      useAuthStore.setState({ accessToken: res.access_token, isAuthenticated: true })
      return res.access_token
    } catch {
      useAuthStore.setState({ user: null, accessToken: null, isAuthenticated: false })
      return null
    } finally {
      refreshPromise = null
    }
  })()
  return refreshPromise
}

setTokenExpiredHandler(refreshAccessToken)
