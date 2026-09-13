import { describe, expect, it } from 'vitest'
import { cleanParams, formatTime } from './index'

describe('cleanParams', () => {
  it('removes empty values while keeping zero and false', () => {
    expect(cleanParams({ page: 1, keyword: '', enabled: false, count: 0, missing: undefined })).toEqual({
      page: 1,
      enabled: false,
      count: 0,
    })
  })
})

describe('formatTime', () => {
  it('formats RFC3339 to seconds', () => {
    expect(formatTime('2026-09-13T20:30:45.123+08:00')).toBe('2026-09-13 20:30:45')
    expect(formatTime(null)).toBe('-')
  })
})
