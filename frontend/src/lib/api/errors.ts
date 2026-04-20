/**
 * Typed API error thrown by the HTTP client.
 *
 * Every non-2xx response is converted into an `ApiError` so that UI code
 * never encounters raw `Error` or untyped strings.
 *
 * - `status`    — HTTP status code (e.g. 400, 404, 500).
 * - `code`      — machine-readable error code from the server (e.g. "DUPLICATE_SLUG"),
 *                 or `HTTP_{status}` if the server didn't provide one.
 * - `message`   — human-readable description from the server.
 * - `requestId` — X-Request-ID header for correlating with server logs.
 */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }

  /** 401 — session expired or missing auth cookie. */
  isUnauthorized(): boolean {
    return this.status === 401
  }
  /** 403 — authenticated but lacks required role/permission. */
  isForbidden(): boolean {
    return this.status === 403
  }
  /** 404 — resource does not exist or was deleted. */
  isNotFound(): boolean {
    return this.status === 404
  }
  /** 409 — optimistic concurrency conflict (stale _version). */
  isConflict(): boolean {
    return this.status === 409
  }
  /** 400 or 422 — request payload failed server-side validation. */
  isValidation(): boolean {
    return this.status === 400 || this.status === 422
  }
  /** 5xx — server-side fault, may be retried. */
  isServer(): boolean {
    return this.status >= 500
  }
}

/** Korean user-friendly fallback messages keyed by HTTP status code. */
const FRIENDLY_MESSAGES: Record<number, string> = {
  400: '입력값을 확인해 주세요.',
  401: '로그인이 필요합니다. 다시 로그인해 주세요.',
  403: '이 작업을 수행할 권한이 없습니다.',
  404: '요청한 항목을 찾을 수 없습니다.',
  409: '다른 사용자가 수정한 내용과 충돌합니다. 새로고침 후 다시 시도해 주세요.',
  413: '파일 크기가 너무 큽니다.',
  422: '입력값을 확인해 주세요.',
  429: '요청이 너무 많습니다. 잠시 후 다시 시도해 주세요.',
  500: '서버에 문제가 발생했습니다. 잠시 후 다시 시도해 주세요.',
  502: '서버에 연결할 수 없습니다. 잠시 후 다시 시도해 주세요.',
  503: '서버 점검 중입니다. 잠시 후 다시 시도해 주세요.',
}

/**
 * Format any error into a Korean user-friendly message for toasts or inline display.
 *
 * Resolution order:
 * 1. `ApiError` — use server message if meaningful, else status-based Korean fallback.
 * 2. Native `Error` — use message (with special handling for "Failed to fetch" -> network hint).
 * 3. String — pass through.
 * 4. Unknown — generic fallback.
 *
 * Always returns a non-empty string so callers don't need to null-check.
 */
export function formatError(err: unknown): string {
  if (err instanceof ApiError) {
    // Use the server message if it's meaningful, otherwise fall back to a
    // friendly status-based message.
    if (err.message && err.message !== err.code) return err.message
    return FRIENDLY_MESSAGES[err.status] ?? err.message
  }
  if (err instanceof Error) {
    if (err.message === 'Failed to fetch') return '네트워크 연결을 확인해 주세요.'
    return err.message
  }
  if (typeof err === 'string') return err
  return '알 수 없는 오류가 발생했습니다.'
}
