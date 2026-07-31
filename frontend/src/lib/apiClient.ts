/**
 * Typed fetch wrapper — the one place the frontend talks HTTP to the Go API
 * (documentation/08-frontend-architecture.md §2, "apiClient — typed fetch
 * wrapper"). Feature code calls apiClient.get/post/etc.; it never calls
 * fetch directly, so the error contract, auth header, and silent-refresh
 * retry live in exactly one place.
 */

const API_BASE_URL: string = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1'

/** Mirrors internal/platform/errors.ProblemDetails on the backend exactly —
 * see documentation/07-api-specification.md §1.3. */
export interface ProblemDetails {
  type: string
  title: string
  status: number
  detail: string
  instance?: string
  code: string
  request_id?: string
  errors?: { field: string; message: string }[]
}

/** Thrown for any non-2xx response. `problem.code` is what calling code
 * should switch on — never `title` or `detail`, which are prose. */
export class ApiError extends Error {
  problem: ProblemDetails

  constructor(problem: ProblemDetails) {
    super(problem.detail)
    this.name = 'ApiError'
    this.problem = problem
  }
}

/** Set by the auth store once a token exists (documentation/08-frontend-architecture.md,
 * Zustand holds auth state). Kept here as a plain module-level getter,
 * rather than importing the store directly, so apiClient has no dependency
 * on the auth feature — the auth feature depends on apiClient, not the
 * other way around. */
let getAccessToken: () => string | null = () => null

export function setAccessTokenGetter(fn: () => string | null): void {
  getAccessToken = fn
}

/** Set by the auth store: given a 401 `auth.token_expired`, attempt a
 * refresh and return the new access token, or null if the session is dead.
 * The store owns single-flight de-duplication (documentation/08's
 * `Token storage + refresh with single-flight — never fire ten concurrent
 * refreshes`) — apiClient just calls whatever this resolves to. */
let onTokenExpired: (() => Promise<string | null>) | null = null

export function setTokenExpiredHandler(fn: () => Promise<string | null>): void {
  onTokenExpired = fn
}

async function parseProblem(response: Response): Promise<ProblemDetails> {
  const isJson = response.headers.get('content-type')?.includes('json')
  if (isJson) {
    try {
      return (await response.json()) as ProblemDetails
    } catch {
      // fall through to the generic problem below
    }
  }
  return {
    type: 'about:blank',
    title: response.statusText,
    status: response.status,
    detail: 'The server returned an unexpected response.',
    code: 'unknown',
  }
}

async function request<T>(path: string, init: RequestInit = {}, isRetry = false): Promise<T> {
  const token = getAccessToken()
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  // credentials: 'include' is required for the gp_refresh cookie to travel
  // on /auth/refresh — the frontend (5173) and API (8080) are different
  // origins in dev, so cookies aren't sent by default
  // (documentation/07-api-specification.md §2, "Token lifecycle").
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: 'include',
  })

  if (response.status === 204) {
    return undefined as T
  }

  if (!response.ok) {
    const problem = await parseProblem(response)

    if (problem.code === 'auth.token_expired' && !isRetry && onTokenExpired) {
      const newToken = await onTokenExpired()
      if (newToken) {
        return request<T>(path, init, true)
      }
    }

    throw new ApiError(problem)
  }

  const isJson = response.headers.get('content-type')?.includes('json')
  return (isJson ? await response.json() : undefined) as T
}

export const apiClient = {
  get: <T>(path: string) => request<T>(path, { method: 'GET' }),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'POST',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'PUT',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'PATCH',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
