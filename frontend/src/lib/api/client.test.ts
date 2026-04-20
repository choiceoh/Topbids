import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { api, type ListEnvelope } from './client'
import { ApiError } from './errors'

// Mock fetch globally.
const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
  // Stub window.location
  Object.defineProperty(window, 'location', {
    value: { pathname: '/', href: '' },
    writable: true,
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

function jsonResponse(body: unknown, status = 200, headers?: Record<string, string>) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    headers: {
      get: (key: string) => headers?.[key] ?? null,
    },
    json: () => Promise.resolve(body),
  })
}

describe('api.get', () => {
  it('unwraps envelope and returns data', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({ data: { id: '1', name: 'Test' } }))

    const result = await api.get<{ id: string; name: string }>('/collections/1')
    expect(result).toEqual({ id: '1', name: 'Test' })
    expect(mockFetch).toHaveBeenCalledWith('/api/collections/1', expect.objectContaining({
      method: 'GET',
      credentials: 'include',
    }))
  })

  it('returns bare value when no envelope', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({ id: '1' }))
    const result = await api.get<{ id: string }>('/test')
    expect(result).toEqual({ id: '1' })
  })

  it('returns undefined for 204', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: true,
        status: 204,
        headers: { get: () => null },
      }),
    )
    const result = await api.del('/test')
    expect(result).toBeUndefined()
  })
})

describe('api.getList', () => {
  it('returns raw list envelope', async () => {
    const envelope: ListEnvelope<{ id: string }> = {
      data: [{ id: '1' }],
      total: 1,
      page: 1,
      limit: 20,
      total_pages: 1,
    }
    mockFetch.mockReturnValueOnce(jsonResponse(envelope))

    const result = await api.getList<{ id: string }>('/entries')
    expect(result.data).toHaveLength(1)
    expect(result.total).toBe(1)
  })
})

describe('api.post', () => {
  it('sends JSON body', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({ data: { id: '2' } }))

    await api.post('/collections', { slug: 'tasks', label: 'Tasks' })

    expect(mockFetch).toHaveBeenCalledWith('/api/collections', expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ slug: 'tasks', label: 'Tasks' }),
    }))
  })
})

describe('error handling', () => {
  it('throws ApiError on non-2xx', async () => {
    mockFetch.mockReturnValueOnce(
      jsonResponse(
        { code: 'NOT_FOUND', message: '찾을 수 없습니다' },
        404,
        { 'X-Request-ID': 'req-abc' },
      ),
    )

    await expect(api.get('/missing')).rejects.toThrow(ApiError)

    try {
      mockFetch.mockReturnValueOnce(
        jsonResponse({ code: 'NOT_FOUND', message: '찾을 수 없습니다' }, 404, { 'X-Request-ID': 'req-abc' }),
      )
      await api.get('/missing')
    } catch (e) {
      const err = e as ApiError
      expect(err.status).toBe(404)
      expect(err.code).toBe('NOT_FOUND')
      expect(err.message).toBe('찾을 수 없습니다')
      expect(err.requestId).toBe('req-abc')
    }
  })

  it('falls back to HTTP status text when body is not JSON', async () => {
    mockFetch.mockReturnValueOnce(
      Promise.resolve({
        ok: false,
        status: 502,
        statusText: 'Bad Gateway',
        headers: { get: () => null },
        json: () => Promise.reject(new Error('not json')),
      }),
    )

    await expect(api.get('/broken')).rejects.toThrow('Bad Gateway')
  })

  it('redirects to /login on 401', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({ message: 'unauthorized' }, 401))

    try {
      await api.get('/protected')
    } catch {
      // expected
    }

    expect(window.location.href).toBe('/login')
  })
})
