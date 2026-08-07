import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { resetAllHints } from '../composables/useFirstVisitHint'

const STORAGE_KEY = 'papeer-onboarding'
const LEGACY_STORAGE_KEY = 'apepa-onboarding' // pre-rebrand, migrated on first read

interface PersistedState {
  completedPhases: number[]
  skippedAll: boolean
  demoProfileId: number | null
}

function loadState(): PersistedState {
  try {
    // Migrate pre-rebrand key: move value to the new key once.
    const legacy = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (legacy !== null && localStorage.getItem(STORAGE_KEY) === null) {
      localStorage.setItem(STORAGE_KEY, legacy)
    }
    if (legacy !== null) {
      localStorage.removeItem(LEGACY_STORAGE_KEY)
    }
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      return {
        completedPhases: Array.isArray(parsed.completedPhases) ? parsed.completedPhases : [],
        skippedAll: !!parsed.skippedAll,
        demoProfileId: parsed.demoProfileId ?? null,
      }
    }
  } catch {
    // corrupted — treat as first launch
  }
  return { completedPhases: [], skippedAll: false, demoProfileId: null }
}

export const useOnboardingStore = defineStore('onboarding', () => {
  const hasStorageKey = localStorage.getItem(STORAGE_KEY) !== null
  const initial = loadState()

  const completedPhases = ref(new Set<number>(initial.completedPhases))
  const skippedAll = ref(initial.skippedAll)
  const activePhase = ref<number | null>(null)
  const demoProfileId = ref<number | null>(initial.demoProfileId)

  const isFirstLaunch = computed(() => !hasStorageKey && completedPhases.value.size === 0 && !skippedAll.value)
  const isDemoMode = computed(() => demoProfileId.value !== null)

  function persist() {
    const state: PersistedState = {
      completedPhases: [...completedPhases.value],
      skippedAll: skippedAll.value,
      demoProfileId: demoProfileId.value,
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  }

  function shouldShowPhase(phase: number): boolean {
    return !skippedAll.value && !completedPhases.value.has(phase)
  }

  function markPhaseComplete(phase: number) {
    completedPhases.value.add(phase)
    completedPhases.value = new Set(completedPhases.value) // trigger reactivity
    persist()
  }

  function skipAll() {
    skippedAll.value = true
    persist()
  }

  function setDemoProfileId(id: number | null) {
    demoProfileId.value = id
    persist()
  }

  const pendingRestart = ref(false)

  function restart() {
    completedPhases.value = new Set<number>()
    skippedAll.value = false
    demoProfileId.value = null
    pendingRestart.value = true
    resetAllHints()
    persist()
  }

  return {
    completedPhases,
    skippedAll,
    activePhase,
    demoProfileId,
    pendingRestart,
    isFirstLaunch,
    isDemoMode,
    shouldShowPhase,
    markPhaseComplete,
    skipAll,
    setDemoProfileId,
    restart,
    persist,
  }
})
