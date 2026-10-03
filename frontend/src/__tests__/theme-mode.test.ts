import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resolveThemeMode, loadThemePreference, setThemePreference, themePreference } from '../theme/mode'
import { getThemeOverrides } from '../theme/naive-overrides'

describe('resolveThemeMode', () => {
  it('honours an explicit preference regardless of the OS', () => {
    expect(resolveThemeMode('light', 'dark')).toBe('light')
    expect(resolveThemeMode('dark', 'light')).toBe('dark')
  })

  it('follows the OS in auto mode', () => {
    expect(resolveThemeMode('auto', 'light')).toBe('light')
    expect(resolveThemeMode('auto', 'dark')).toBe('dark')
  })

  it('falls back to dark when the OS theme is unknown', () => {
    expect(resolveThemeMode('auto', null)).toBe('dark')
    expect(resolveThemeMode('auto', undefined)).toBe('dark')
  })
})

describe('theme preference storage', () => {
  beforeEach(() => localStorage.clear())

  it('defaults to auto and ignores garbage', () => {
    expect(loadThemePreference()).toBe('auto')
    localStorage.setItem('papeer-theme', 'purple')
    expect(loadThemePreference()).toBe('auto')
  })

  it('persists the chosen preference', () => {
    setThemePreference('light')
    expect(themePreference.value).toBe('light')
    expect(loadThemePreference()).toBe('light')
  })

  it('survives a throwing localStorage', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('denied') })
    const setSpy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('denied') })
    expect(loadThemePreference()).toBe('auto')
    expect(() => setThemePreference('dark')).not.toThrow()
    spy.mockRestore()
    setSpy.mockRestore()
  })
})

describe('getThemeOverrides', () => {
  it('keeps the dark palette and swaps surfaces for light', () => {
    expect(getThemeOverrides('dark').common?.bodyColor).toBe('#0f172a')
    expect(getThemeOverrides('light').common?.bodyColor).toBe('#f8fafc')
    // Shared, non-color settings carry over to the light theme.
    expect(getThemeOverrides('light').common?.primaryColor).toBe('#7c5cff')
    expect(getThemeOverrides('light').Card?.borderRadius).toBe('12px')
  })
})
