import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../../wailsjs/go/app/App', () => ({
  ListPapers: vi.fn(),
}))

import { ListPapers } from '../../wailsjs/go/app/App'
import { fetchApprovedDownloadedMap } from '../utils/paperMap'

describe('fetchApprovedDownloadedMap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('merges approved and downloaded papers into one id -> paper map', async () => {
    vi.mocked(ListPapers)
      .mockResolvedValueOnce({ papers: [{ id: 1, title: 'Approved One' }], total: 1 } as any)
      .mockResolvedValueOnce({ papers: [{ id: 2, title: 'Downloaded One' }], total: 1 } as any)

    const map = await fetchApprovedDownloadedMap(42)

    expect(map.size).toBe(2)
    expect(map.get(1)?.title).toBe('Approved One')
    expect(map.get(2)?.title).toBe('Downloaded One')
  })

  it('requests both statuses scoped to the given profile id', async () => {
    vi.mocked(ListPapers).mockResolvedValue({ papers: [], total: 0 } as any)

    await fetchApprovedDownloadedMap(7)

    expect(ListPapers).toHaveBeenCalledTimes(2)
    const calledStatuses = vi.mocked(ListPapers).mock.calls.map((c) => (c[0] as any).status)
    expect(calledStatuses.sort()).toEqual(['approved', 'downloaded'])
    for (const call of vi.mocked(ListPapers).mock.calls) {
      expect((call[0] as any).profile_id).toBe(7)
    }
  })

  it('handles null returns from the backend (Go nil slice -> JS null)', async () => {
    vi.mocked(ListPapers).mockResolvedValue({ papers: null, total: 0 } as any)

    const map = await fetchApprovedDownloadedMap(1)
    expect(map.size).toBe(0)
  })

  it('last write wins when the same id appears in both lists', async () => {
    // Shouldn't normally happen (a paper has one status), but the merge
    // logic shouldn't crash if it does.
    vi.mocked(ListPapers)
      .mockResolvedValueOnce({ papers: [{ id: 1, title: 'First' }], total: 1 } as any)
      .mockResolvedValueOnce({ papers: [{ id: 1, title: 'Second' }], total: 1 } as any)

    const map = await fetchApprovedDownloadedMap(1)
    expect(map.size).toBe(1)
    expect(map.get(1)?.title).toBe('Second')
  })
})
