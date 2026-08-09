import { describe, it, expect } from 'vitest'

/**
 * Pure-function versions of the contextual button logic from PaperDetailPanel.
 * statusFilter values: '' | 'new' | 'approved' | 'rejected' | 'downloaded'
 */
function shouldShowApprove(statusFilter: string): boolean {
  if (!statusFilter || statusFilter === 'new') return true
  if (statusFilter === 'rejected') return true
  return false
}

function shouldShowReject(statusFilter: string): boolean {
  if (!statusFilter || statusFilter === 'new') return true
  if (statusFilter === 'approved') return true
  if (statusFilter === 'downloaded') return true
  return false
}

describe('Contextual action buttons', () => {
  describe('showApproveButton', () => {
    it('visible on "all" tab', () => {
      expect(shouldShowApprove('')).toBe(true)
    })
    it('visible on "new" tab', () => {
      expect(shouldShowApprove('new')).toBe(true)
    })
    it('hidden on "approved" tab', () => {
      expect(shouldShowApprove('approved')).toBe(false)
    })
    it('visible on "rejected" tab', () => {
      expect(shouldShowApprove('rejected')).toBe(true)
    })
    it('hidden on "downloaded" tab', () => {
      expect(shouldShowApprove('downloaded')).toBe(false)
    })
  })

  describe('showRejectButton', () => {
    it('visible on "all" tab', () => {
      expect(shouldShowReject('')).toBe(true)
    })
    it('visible on "new" tab', () => {
      expect(shouldShowReject('new')).toBe(true)
    })
    it('visible on "approved" tab', () => {
      expect(shouldShowReject('approved')).toBe(true)
    })
    it('hidden on "rejected" tab', () => {
      expect(shouldShowReject('rejected')).toBe(false)
    })
    it('visible on "downloaded" tab', () => {
      expect(shouldShowReject('downloaded')).toBe(true)
    })
  })
})
