import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const handlers: Record<string, (data: any) => void> = {}
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, fn: (data: any) => void) => { handlers[name] = fn },
  EventsOff: vi.fn(),
}))

import { useProgressStore, MAX_PROGRESS_EVENTS } from '../stores/progress'

describe('progress store event caps', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('keeps only the latest search events', () => {
    const store = useProgressStore()
    store.startListening()
    for (let i = 0; i < MAX_PROGRESS_EVENTS + 50; i++) {
      handlers['search:progress']({ type: 'query_start', query: `q${i}` })
    }
    expect(store.searchEvents).toHaveLength(MAX_PROGRESS_EVENTS)
    expect(store.searchEvents[0].query).toBe('q50')
    expect(store.searchEvents.at(-1)!.query).toBe(`q${MAX_PROGRESS_EVENTS + 49}`)
  })

  it('caps download events but still counts every done/fail', () => {
    const store = useProgressStore()
    const notify = vi.fn()
    store.setNotify(notify)
    store.startListening()
    const n = MAX_PROGRESS_EVENTS
    for (let i = 1; i <= n; i++) {
      handlers['download:progress']({ type: 'start', paper_id: i, current: i, total: n })
      handlers['download:progress']({ type: i % 5 === 0 ? 'fail' : 'done', paper_id: i, current: i, total: n })
    }
    expect(store.downloadEvents).toHaveLength(MAX_PROGRESS_EVENTS)
    expect(store.downloadDone).toBe(n - n / 5)
    expect(store.downloadFailed).toBe(n / 5)

    handlers['download:done'](null)
    expect(notify).toHaveBeenCalledWith('warning', `Download complete: ${n - n / 5} downloaded, ${n / 5} failed`)
  })
})
