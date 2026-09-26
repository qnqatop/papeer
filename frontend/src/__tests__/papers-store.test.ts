import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('../../wailsjs/go/app/App', () => ({
  ListPapers: vi.fn(),
  GetPaper: vi.fn(),
  UpdatePaperStatus: vi.fn(),
  BulkUpdateStatus: vi.fn(),
  SetUserScore: vi.fn(),
  SetPaperNotes: vi.fn(),
  ApproveByScore: vi.fn(),
}))

import { ListPapers } from '../../wailsjs/go/app/App'
import { usePapersStore } from '../stores/papers'

function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}

describe('papers store fetchPapers', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('ignores a stale response that resolves after a newer one', async () => {
    const slow = deferred<any>()
    const fast = deferred<any>()
    vi.mocked(ListPapers)
      .mockReturnValueOnce(slow.promise)
      .mockReturnValueOnce(fast.promise)

    const store = usePapersStore()
    store.filter.profile_id = 1

    const first = store.fetchPapers()   // old filter, slow backend
    const second = store.fetchPapers()  // new filter, fast backend

    fast.resolve({ papers: [{ id: 2 }], total: 1 })
    await second
    expect(store.papers.map((p: any) => p.id)).toEqual([2])
    expect(store.loading).toBe(false)

    slow.resolve({ papers: [{ id: 1 }, { id: 3 }], total: 2 })
    await first
    expect(store.papers.map((p: any) => p.id)).toEqual([2])
    expect(store.total).toBe(1)
    expect(store.loading).toBe(false)
  })

  it('keeps loading until the latest request finishes', async () => {
    const slow = deferred<any>()
    const fast = deferred<any>()
    vi.mocked(ListPapers)
      .mockReturnValueOnce(fast.promise)
      .mockReturnValueOnce(slow.promise)

    const store = usePapersStore()
    store.filter.profile_id = 1

    const first = store.fetchPapers()
    const second = store.fetchPapers()

    fast.resolve({ papers: [{ id: 1 }], total: 1 })
    await first
    expect(store.loading).toBe(true)
    expect(store.papers).toEqual([])

    slow.resolve({ papers: [{ id: 2 }], total: 1 })
    await second
    expect(store.loading).toBe(false)
    expect(store.papers.map((p: any) => p.id)).toEqual([2])
  })
})
