/**
 * HTTP client for the Topworks API.
 *
 * All endpoints return a `{data, error}` envelope. This client unwraps the
 * envelope: successful calls return `T` directly, failures throw `ApiError`.
 *
 * Features:
 * - Automatic 401 -> redirect to /login (single auth fallback)
 * - 30s request timeout via AbortSignal
 * - credentials: 'include' for httpOnly cookie auth
 * - Type-safe generic methods: get<T>, post<T>, patch<T>, put<T>, del<T>
 */

import { ApiError } from './errors'

const BASE = '/api'
const DEFAULT_TIMEOUT_MS = 30_000

interface Envelope<T> {
  data?: T
  error?: string
}

interface ListEnvelope<T> {
  data: T[]
  total: number
  page: number
  limit: number
  total_pages: number
}

export type { ListEnvelope }

interface RequestOptions {
  signal?: AbortSignal
  // When true, the response is returned as-is (no envelope unwrap). Used for
  // list endpoints that include total/page metadata at the envelope level.
  raw?: boolean
  // Per-request timeout in ms. Defaults to DEFAULT_TIMEOUT_MS (30s).
  timeout?: number
}

/**
 * Core fetch wrapper. Sends a JSON request, unwraps the `{data}` envelope,
 * and throws {@link ApiError} on non-2xx responses. On 401, redirects the
 * browser to /login. Returns `undefined` for 204 No Content. When `opts.raw`
 * is true, skips envelope unwrapping (used by `getList` for paginated responses).
 */
async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts: RequestOptions = {},
): Promise<T> {
  const timeoutMs = opts.timeout ?? DEFAULT_TIMEOUT_MS
  const timeoutSignal = AbortSignal.timeout(timeoutMs)
  const signal = opts.signal
    ? AbortSignal.any([opts.signal, timeoutSignal])
    : timeoutSignal

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'include',
    signal,
  })

  const requestId = res.headers.get('X-Request-ID') ?? undefined

  if (!res.ok) {
    let message = res.statusText || `HTTP ${res.status}`
    let code = `HTTP_${res.status}`
    try {
      const parsed = await res.json()
      message = parsed.message ?? parsed.error ?? message
      code = parsed.code ?? code
    } catch {
      // body wasn't JSON — fall through with HTTP defaults
    }

    if (res.status === 401 && !window.location.pathname.startsWith('/login')) {
      window.location.href = '/login'
    }

    throw new ApiError(res.status, code, message, requestId)
  }

  if (res.status === 204) return undefined as T

  const json = (await res.json()) as Envelope<T> | T

  if (opts.raw) return json as T

  // Envelope unwrap. We accept both `{data: ...}` and bare values for
  // backwards compatibility with hand-written API endpoints.
  if (json && typeof json === 'object' && 'data' in (json as Envelope<T>)) {
    return (json as Envelope<T>).data as T
  }
  return json as T
}

/** Response from a successful file upload. */
export interface UploadResult {
  url: string
  name: string
  size: number
}

/**
 * Upload a file via multipart/form-data to `/api/upload`.
 * Does NOT set Content-Type manually — the browser adds the boundary automatically.
 */
async function uploadFile(file: File): Promise<UploadResult> {
  const form = new FormData()
  form.append('file', file)

  const res = await fetch(`${BASE}/upload`, {
    method: 'POST',
    body: form,
    credentials: 'include',
  })

  if (!res.ok) {
    let message = res.statusText || `HTTP ${res.status}`
    try {
      const parsed = await res.json()
      message = parsed.message ?? parsed.error ?? message
    } catch {
      // body wasn't JSON
    }
    throw new ApiError(res.status, `HTTP_${res.status}`, message)
  }

  const json = (await res.json()) as Envelope<UploadResult>
  return json.data as UploadResult
}

export const api = {
  /** GET a single resource; envelope-unwrapped to `T`. */
  get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, undefined, opts),
  /**
   * GET a paginated list. Returns the full {@link ListEnvelope} with
   * `data` (array), `total`, `page`, `limit`, and `total_pages`.
   * Defaults `data` to `[]` if the server returns null.
   */
  getList: <T>(path: string, opts?: RequestOptions) =>
    request<ListEnvelope<T>>('GET', path, undefined, { ...opts, raw: true }).then(
      (res) => ({ ...res, data: res.data ?? [] }),
    ),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('POST', path, body, opts),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('PATCH', path, body, opts),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('PUT', path, body, opts),
  del: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('DELETE', path, body, opts),
  /** Upload a file via multipart/form-data. Returns the stored URL, name, and size. */
  upload: uploadFile,
}
