import { describe, it, expect } from 'vitest'

/**
 * Unit tests for the visibility logic used in AppShell.vue.
 * These test the computed conditions that determine what the user sees.
 */
describe('AppShell visibility logic', () => {
  // Simulates the v-if condition: profileStore.profiles.length === 0 && !profileStore.loading
  function shouldShowWelcomeCard(profiles: any[] | null | undefined, loading: boolean): boolean {
    const profilesArr = profiles || []
    return profilesArr.length === 0 && !loading
  }

  // Simulates the menuOptions condition
  function getMenuOptions(profiles: any[] | null | undefined): string[] {
    const profilesArr = profiles || []
    if (profilesArr.length === 0) return []
    return ['search', 'papers', 'analysis']
  }

  describe('WelcomeCard visibility', () => {
    it('shows WelcomeCard when profiles is empty and not loading', () => {
      expect(shouldShowWelcomeCard([], false)).toBe(true)
    })

    it('shows WelcomeCard when profiles is null (Go nil slice)', () => {
      expect(shouldShowWelcomeCard(null, false)).toBe(true)
    })

    it('shows WelcomeCard when profiles is undefined', () => {
      expect(shouldShowWelcomeCard(undefined, false)).toBe(true)
    })

    it('hides WelcomeCard while loading', () => {
      expect(shouldShowWelcomeCard([], true)).toBe(false)
      expect(shouldShowWelcomeCard(null, true)).toBe(false)
    })

    it('hides WelcomeCard when profiles exist', () => {
      expect(shouldShowWelcomeCard([{ id: 1 }], false)).toBe(false)
    })
  })

  describe('menu visibility', () => {
    it('returns empty menu when no profiles', () => {
      expect(getMenuOptions([])).toEqual([])
    })

    it('returns empty menu when profiles is null', () => {
      expect(getMenuOptions(null)).toEqual([])
    })

    it('returns full menu when profiles exist', () => {
      expect(getMenuOptions([{ id: 1 }])).toEqual(['search', 'papers', 'analysis'])
    })
  })
})
