import { describe, it, expect } from 'vitest'
import { willLeaveStatusFilter } from '../utils/statusFilter'

describe('willLeaveStatusFilter', () => {
  it('keeps the paper on the "All" tab (null or empty filter)', () => {
    expect(willLeaveStatusFilter(null, 'approved')).toBe(false)
    expect(willLeaveStatusFilter('', 'rejected')).toBe(false)
    expect(willLeaveStatusFilter(undefined, 'new')).toBe(false)
  })

  it('drops the paper when the new status differs from the filter', () => {
    expect(willLeaveStatusFilter('new', 'approved')).toBe(true)
  })

  it('keeps the paper when the status matches the filter', () => {
    expect(willLeaveStatusFilter('approved', 'approved')).toBe(false)
  })
})
