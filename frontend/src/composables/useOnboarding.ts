import { watch, nextTick, onUnmounted } from 'vue'
import { driver } from 'driver.js'
import type { DriveStep, Driver } from 'driver.js'
import { useI18n } from 'vue-i18n'
import { useOnboardingStore } from '../stores/onboarding'

export function useOnboardingPhase(phase: number, options: {
  precondition: () => boolean
  getSteps: () => DriveStep[]
  onComplete?: () => void
}) {
  const store = useOnboardingStore()
  const { t } = useI18n()
  let driverInstance: Driver | null = null
  let started = false

  function startDriver() {
    if (driverInstance || started) return
    if (!store.shouldShowPhase(phase)) return

    started = true
    store.activePhase = phase

    const steps = options.getSteps()
    if (steps.length === 0) return

    driverInstance = driver({
      animate: true,
      overlayColor: 'rgba(0, 0, 0, 0.75)',
      stagePadding: 8,
      stageRadius: 8,
      allowClose: true,
      disableActiveInteraction: false,
      popoverClass: 'papeer-onboarding-popover',
      nextBtnText: t('onboarding.next'),
      prevBtnText: t('onboarding.prev'),
      doneBtnText: t('onboarding.done'),
      steps,
      onDestroyStarted: () => {
        store.markPhaseComplete(phase)
        store.activePhase = null
        driverInstance?.destroy()
        driverInstance = null
        options.onComplete?.()
      },
    })

    driverInstance.drive()
  }

  function destroy() {
    if (driverInstance) {
      // Mark complete even if dismissed mid-tour
      store.markPhaseComplete(phase)
      store.activePhase = null
      driverInstance.destroy()
      driverInstance = null
    }
  }

  // Watch precondition — trigger when it becomes true
  const stopWatch = watch(
    () => options.precondition() && store.shouldShowPhase(phase) && store.activePhase === null,
    async (ready) => {
      if (ready && !started) {
        await nextTick()
        // Small delay for Naive UI render transitions
        setTimeout(startDriver, 150)
      }
    },
    { flush: 'post', immediate: true },
  )

  // Also re-trigger when completedPhases changes (for restart)
  const stopRestartWatch = watch(
    () => [...store.completedPhases],
    () => {
      if (store.shouldShowPhase(phase) && options.precondition() && !started && store.activePhase === null) {
        started = false // allow re-trigger after restart
        nextTick(() => setTimeout(startDriver, 150))
      }
    },
  )

  onUnmounted(() => {
    stopWatch()
    stopRestartWatch()
    destroy()
  })

  return { destroy }
}
