/**
 * Typed fetch wrapper — the one place the frontend talks HTTP to the Go API
 * (documentation/08-frontend-architecture.md §2, "apiClient — typed fetch
 * wrapper"). Feature code calls apiClient.get/post/etc.; it never calls
 * fetch directly, so the error contract and auth header attachment live in
 * exactly one place.
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

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getAccessToken()
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers })

  if (response.status === 204) {
    return undefined as T
  }

  const isJson = response.headers.get('content-type')?.includes('json')
  const body = isJson ? await response.json() : undefined

  if (!response.ok) {
    const problem: ProblemDetails = body ?? {
      type: 'about:blank',
      title: response.statusText,
      status: response.status,
      detail: 'The server returned an unexpected response.',
      code: 'unknown',
    }
    throw new ApiError(problem)
  }

  return body as T
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
