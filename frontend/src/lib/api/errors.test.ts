import { describe, it, expect } from 'vitest'
import { ApiError, formatError } from './errors'

describe('ApiError', () => {
  it('sets name, status, code, message, requestId', () => {
    const err = new ApiError(404, 'NOT_FOUND', 'resource not found', 'req-123')
    expect(err.name).toBe('ApiError')
    expect(err.status).toBe(404)
    expect(err.code).toBe('NOT_FOUND')
    expect(err.message).toBe('resource not found')
    expect(err.requestId).toBe('req-123')
  })

  it('extends Error', () => {
    const err = new ApiError(500, 'INTERNAL', 'oops')
    expect(err).toBeInstanceOf(Error)
    expect(err).toBeInstanceOf(ApiError)
  })

  it('requestId is optional', () => {
    const err = new ApiError(400, 'BAD_REQUEST', 'bad')
    expect(err.requestId).toBeUndefined()
  })

  describe('status checkers', () => {
    it('isUnauthorized', () => {
      expect(new ApiError(401, '', '').isUnauthorized()).toBe(true)
      expect(new ApiError(403, '', '').isUnauthorized()).toBe(false)
    })

    it('isForbidden', () => {
      expect(new ApiError(403, '', '').isForbidden()).toBe(true)
      expect(new ApiError(401, '', '').isForbidden()).toBe(false)
    })

    it('isNotFound', () => {
      expect(new ApiError(404, '', '').isNotFound()).toBe(true)
      expect(new ApiError(400, '', '').isNotFound()).toBe(false)
    })

    it('isConflict', () => {
      expect(new ApiError(409, '', '').isConflict()).toBe(true)
      expect(new ApiError(400, '', '').isConflict()).toBe(false)
    })

    it('isValidation for 400 and 422', () => {
      expect(new ApiError(400, '', '').isValidation()).toBe(true)
      expect(new ApiError(422, '', '').isValidation()).toBe(true)
      expect(new ApiError(404, '', '').isValidation()).toBe(false)
    })

    it('isServer for 5xx', () => {
      expect(new ApiError(500, '', '').isServer()).toBe(true)
      expect(new ApiError(503, '', '').isServer()).toBe(true)
      expect(new ApiError(499, '', '').isServer()).toBe(false)
    })
  })
})

describe('formatError', () => {
  it('formats ApiError with meaningful message', () => {
    const err = new ApiError(404, 'NOT_FOUND', '찾을 수 없습니다')
    expect(formatError(err)).toBe('찾을 수 없습니다')
  })

  it('falls back to friendly message when message equals code', () => {
    const err = new ApiError(404, 'NOT_FOUND', 'NOT_FOUND')
    expect(formatError(err)).toBe('요청한 항목을 찾을 수 없습니다.')
  })

  it('falls back to friendly message for empty message', () => {
    const err = new ApiError(500, 'INTERNAL', '')
    expect(formatError(err)).toBe('서버에 문제가 발생했습니다. 잠시 후 다시 시도해 주세요.')
  })

  it('formats network error', () => {
    expect(formatError(new Error('Failed to fetch'))).toBe('네트워크 연결을 확인해 주세요.')
  })

  it('formats regular Error', () => {
    expect(formatError(new Error('something broke'))).toBe('something broke')
  })

  it('formats string', () => {
    expect(formatError('raw string')).toBe('raw string')
  })

  it('formats unknown type', () => {
    expect(formatError(42)).toBe('알 수 없는 오류가 발생했습니다.')
    expect(formatError(null)).toBe('알 수 없는 오류가 발생했습니다.')
    expect(formatError(undefined)).toBe('알 수 없는 오류가 발생했습니다.')
  })
})
