import { ref } from 'vue'

export type ThemePreference = 'auto' | 'light' | 'dark'
export type ThemeMode = 'light' | 'dark'

const THEME_KEY = 'papeer-theme'

/**
 * resolveThemeMode turns the user's preference into the concrete mode.
 * "auto" follows the OS; when the OS gives no answer (null/undefined, e.g.
 * an older webview) the app falls back to its historical dark look.
 */
export function resolveThemeMode(
  pref: ThemePreference,
  osTheme: 'light' | 'dark' | null | undefined,
): ThemeMode {
  if (pref === 'light' || pref === 'dark') return pref
  return osTheme === 'light' ? 'light' : 'dark'
}

function isPreference(v: unknown): v is ThemePreference {
  return v === 'auto' || v === 'light' || v === 'dark'
}

// Storage access can throw (privacy mode, blocked storage); the theme then
// just isn't remembered.
export function loadThemePreference(): ThemePreference {
  try {
    const v = localStorage.getItem(THEME_KEY)
    return isPreference(v) ? v : 'auto'
  } catch {
    return 'auto'
  }
}

/** The user's choice, shared by App.vue and the Settings page. */
export const themePreference = ref<ThemePreference>(loadThemePreference())

/** The applied mode, kept in sync by App.vue for non-CSS consumers (graphs). */
export const resolvedThemeMode = ref<ThemeMode>('dark')

export function setThemePreference(pref: ThemePreference) {
  themePreference.value = pref
  try {
    localStorage.setItem(THEME_KEY, pref)
  } catch {
    /* not persisted */
  }
}
