import { getCurrentInstance, onUnmounted } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'

/**
 * EventsOn that tolerates a missing Wails runtime (Vite-only `make
 * dev-frontend`, tests): the subscription is skipped instead of throwing
 * from component setup and blanking the whole UI.
 */
export function safeEventsOn(name: string, cb: (...data: any) => void): (() => void) | undefined {
  try {
    return EventsOn(name, cb)
  } catch (e) {
    console.warn(`[papeer] cannot subscribe to ${name}:`, e)
    return undefined
  }
}

/**
 * Subscribes to a Wails event and removes exactly this listener when the
 * calling component unmounts. Unlike EventsOff(name), which drops every
 * callback for the event (including other components' and stores'), the
 * unsubscribe function returned by EventsOn only removes our own one.
 *
 * Returns the unsubscribe function so callers can stop listening earlier.
 * Outside a component (e.g. unit tests) no lifecycle hook is registered and
 * the caller owns the returned function.
 */
export function useWailsEvent<T = any>(name: string, cb: (data: T) => void): () => void {
  let active = true
  const off = safeEventsOn(name, cb as (...data: any) => void)
  const unsubscribe = () => {
    if (!active) return
    active = false
    off?.()
  }
  if (getCurrentInstance()) {
    onUnmounted(unsubscribe)
  }
  return unsubscribe
}
