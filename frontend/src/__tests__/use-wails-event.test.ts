import { describe, it, expect, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

const off = vi.fn()
const eventsOn = vi.fn((_name: string, _cb: (...a: unknown[]) => void) => off)
const eventsOff = vi.fn()
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, cb: (...a: unknown[]) => void) => eventsOn(name, cb),
  EventsOff: (...a: unknown[]) => eventsOff(...a),
}))

import { useWailsEvent } from '../composables/useWailsEvent'

describe('useWailsEvent', () => {
  it('subscribes on setup and unsubscribes only its own listener on unmount', () => {
    const cb = vi.fn()
    const Comp = defineComponent({
      setup() {
        useWailsEvent('download:done', cb)
        return () => h('div')
      },
    })
    const wrapper = mount(Comp)
    expect(eventsOn).toHaveBeenCalledWith('download:done', cb)
    expect(off).not.toHaveBeenCalled()
    wrapper.unmount()
    expect(off).toHaveBeenCalledTimes(1)
    expect(eventsOff).not.toHaveBeenCalled()
  })

  it('returned unsubscribe is safe to call more than once', () => {
    off.mockClear()
    const unsub = useWailsEvent('x', () => {})
    unsub()
    unsub()
    expect(off).toHaveBeenCalledTimes(1)
  })
})
