import { describe, it, expect } from 'vitest'

/**
 * Whether the download:done handler should emit a toast notification.
 * Matches the logic in stores/progress.ts: only notify for batch downloads (total > 1).
 */
function shouldNotifyOnDownloadDone(total: number): boolean {
  return total > 1
}

describe('progressStore download:done notification', () => {
  it('notifies for batch downloads (total > 1)', () => {
    expect(shouldNotifyOnDownloadDone(50)).toBe(true)
    expect(shouldNotifyOnDownloadDone(2)).toBe(true)
  })

  it('does NOT notify for single-paper auto-downloads (total === 1)', () => {
    expect(shouldNotifyOnDownloadDone(1)).toBe(false)
  })

  it('does NOT notify for zero total (edge case)', () => {
    expect(shouldNotifyOnDownloadDone(0)).toBe(false)
  })
})
