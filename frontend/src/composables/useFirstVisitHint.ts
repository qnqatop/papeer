import { onMounted, onUnmounted, nextTick, watch, type Ref } from 'vue'
import { driver } from 'driver.js'
import type { DriveStep, Driver } from 'driver.js'
import { useI18n } from 'vue-i18n'
import { useOnboardingStore } from '../stores/onboarding'

const STORAGE_KEY = 'papeer-hints-seen'
const LEGACY_STORAGE_KEY = 'apepa-hints-seen' // pre-rebrand, migrated on first read

function getSeenHints(): Set<string> {
  try {
    const legacy = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (legacy !== null && localStorage.getItem(STORAGE_KEY) === null) {
      localStorage.setItem(STORAGE_KEY, legacy)
    }
    if (legacy !== null) {
      localStorage.removeItem(LEGACY_STORAGE_KEY)
    }
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? new Set(JSON.parse(raw)) : new Set()
  } catch {
    return new Set()
  }
}

function markSeen(id: string) {
  const seen = getSeenHints()
  seen.add(id)
  localStorage.setItem(STORAGE_KEY, JSON.stringify([...seen]))
}

/**
 * Shows a one-time driver.js hint when a tab/view is first visited.
 *
 * @param id       Unique hint identifier (e.g. 'summaries', 'stats')
 * @param options  Steps to show + optional precondition ref
 */
export function useFirstVisitHint(id: string, options: {
  steps: () => DriveStep[]
  /** If provided, waits for this to become true before showing. */
  ready?: Ref<boolean> | (() => boolean)
}) {
  const { t } = useI18n()
  const onboarding = useOnboardingStore()
  let instance: Driver | null = null
  let shown = false

  function tryShow() {
    if (shown) return
    if (getSeenHints().has(id)) return
    // Don't overlap with the main onboarding tour
    if (onboarding.activePhase !== null) return

    const steps = options.steps()
    if (steps.length === 0) return

    shown = true
    markSeen(id)

    instance = driver({
      animate: true,
      overlayColor: 'rgba(0, 0, 0, 0.6)',
      stagePadding: 8,
      stageRadius: 8,
      allowClose: true,
      disableActiveInteraction: false,
      popoverClass: 'papeer-onboarding-popover',
      nextBtnText: t('onboarding.next'),
      prevBtnText: t('onboarding.prev'),
      doneBtnText: t('onboarding.done'),
      steps,
    })

    try {
      instance.drive()
    } catch {
      instance = null
    }
  }

  onMounted(() => {
    const readyVal = options.ready
    if (!readyVal) {
      // No precondition — show after a small delay for DOM to settle
      nextTick(() => setTimeout(tryShow, 300))
      return
    }

    // Watch the precondition
    const getter = typeof readyVal === 'function' ? readyVal : () => readyVal.value
    const stop = watch(getter, (val) => {
      if (val) {
        nextTick(() => setTimeout(tryShow, 300))
        stop()
      }
    }, { immediate: true })

    onUnmounted(stop)
  })

  onUnmounted(() => {
    if (instance) {
      instance.destroy()
      instance = null
    }
  })
}

/** Reset all seen hints (used when restarting onboarding tour). */
export function resetAllHints() {
  localStorage.removeItem(STORAGE_KEY)
}
