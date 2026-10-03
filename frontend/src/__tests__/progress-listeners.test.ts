import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const eventsOn = vi.fn()
const eventsOff = vi.fn()
const unsubs: Array<ReturnType<typeof vi.fn>> = []

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (...args: unknown[]) => {
    eventsOn(...args)
    const off = vi.fn()
    unsubs.push(off)
    return off
  },
  EventsOff: (...args: unknown[]) => eventsOff(...args),
}))

import { useProgressStore } from '../stores/progress'

describe('progress store listener lifecycle', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    eventsOn.mockClear()
    eventsOff.mockClear()
    unsubs.length = 0
  })

  it('stopListening calls its own unsubscribe fns and never EventsOff', () => {
    const store = useProgressStore()
    store.startListening()
    expect(unsubs.length).toBeGreaterThan(0)
    store.stopListening()
    for (const off of unsubs) expect(off).toHaveBeenCalledTimes(1)
    expect(eventsOff).not.toHaveBeenCalled()
  })

  it('startListening is idempotent until stopped', () => {
    const store = useProgressStore()
    store.startListening()
    const first = eventsOn.mock.calls.length
    store.startListening()
    expect(eventsOn.mock.calls.length).toBe(first)
    store.stopListening()
    store.startListening()
    expect(eventsOn.mock.calls.length).toBe(first * 2)
  })
})

describe('progress store failed downloads and radar runs', () => {
  const handlers: Record<string, (data: any) => void> = {}
  beforeEach(() => {
    setActivePinia(createPinia())
    eventsOn.mockImplementation((name: string, fn: (data: any) => void) => { handlers[name] = fn })
  })

  it('tracks failures until the paper downloads or is dismissed', () => {
    const store = useProgressStore()
    store.startListening()
    handlers['download:progress']({ type: 'fail', paper_id: 1, title: 'A', error: 'all sources exhausted', current: 1, total: 2 })
    handlers['download:progress']({ type: 'fail', paper_id: 2, title: 'B', error: 'captcha required', current: 2, total: 2 })
    handlers['download:done'](null)
    expect(store.failedDownloads.map(f => f.paper_id)).toEqual([2, 1])

    // A later successful retry of paper 1 clears it, even in a new run.
    handlers['download:progress']({ type: 'done', paper_id: 1, title: 'A', current: 1, total: 1 })
    expect(store.failedDownloads.map(f => f.paper_id)).toEqual([2])

    store.dismissFailedDownload(2)
    expect(store.failedDownloads).toEqual([])
  })

  it('keeps the latest radar runs', () => {
    const store = useProgressStore()
    store.startListening()
    for (let i = 0; i < 7; i++) handlers['radar:done']({ new_papers: i, profile_id: 1, processed: 10 })
    expect(store.radarRuns).toHaveLength(5)
    expect(store.radarRuns[0].new_papers).toBe(6)
  })
})
