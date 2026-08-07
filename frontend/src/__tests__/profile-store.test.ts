import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useProfileStore } from '../stores/profile'

// Mock Wails bindings
vi.mock('../../wailsjs/go/app/App', () => ({
  ListProfiles: vi.fn(),
  GetProfile: vi.fn(),
  CreateProfile: vi.fn(),
  UpdateProfile: vi.fn(),
  DeleteProfile: vi.fn(),
  SelectDirectory: vi.fn(),
}))

import { ListProfiles, CreateProfile, DeleteProfile } from '../../wailsjs/go/app/App'

describe('profile store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  describe('fetchProfiles', () => {
    it('handles null return from backend (0 profiles)', async () => {
      // Go nil slice → JS null
      vi.mocked(ListProfiles).mockResolvedValue(null as any)

      const store = useProfileStore()
      await store.fetchProfiles()

      expect(store.profiles).toEqual([])
      expect(store.profiles.length).toBe(0)
      expect(store.activeProfileId).toBeNull()
      expect(store.loading).toBe(false)
    })

    it('handles empty array return', async () => {
      vi.mocked(ListProfiles).mockResolvedValue([])

      const store = useProfileStore()
      await store.fetchProfiles()

      expect(store.profiles).toEqual([])
      expect(store.activeProfileId).toBeNull()
      expect(store.loading).toBe(false)
    })

    it('auto-selects first profile when none is active', async () => {
      vi.mocked(ListProfiles).mockResolvedValue([
        { id: 1, name: 'Test', email: '', pdf_dir: '', year_min: 2020, max_per_query: 25 },
      ] as any)

      const store = useProfileStore()
      await store.fetchProfiles()

      expect(store.activeProfileId).toBe(1)
      expect(store.activeProfile?.name).toBe('Test')
    })

    it('preserves active profile if it still exists', async () => {
      vi.mocked(ListProfiles).mockResolvedValue([
        { id: 1, name: 'First', email: '', pdf_dir: '', year_min: 2020, max_per_query: 25 },
        { id: 2, name: 'Second', email: '', pdf_dir: '', year_min: 2020, max_per_query: 25 },
      ] as any)

      const store = useProfileStore()
      store.activeProfileId = 2
      await store.fetchProfiles()

      expect(store.activeProfileId).toBe(2)
    })

    it('falls back to first profile if active was deleted', async () => {
      vi.mocked(ListProfiles).mockResolvedValue([
        { id: 1, name: 'First', email: '', pdf_dir: '', year_min: 2020, max_per_query: 25 },
      ] as any)

      const store = useProfileStore()
      store.activeProfileId = 99 // deleted profile
      await store.fetchProfiles()

      expect(store.activeProfileId).toBe(1)
    })

    it('sets loading = false even on error', async () => {
      vi.mocked(ListProfiles).mockRejectedValue(new Error('DB error'))

      const store = useProfileStore()
      await expect(store.fetchProfiles()).rejects.toThrow('DB error')

      expect(store.loading).toBe(false)
    })
  })

  describe('createProfile', () => {
    it('creates profile and sets it as active', async () => {
      const created = { id: 5, name: 'New', email: '', pdf_dir: '', year_min: 2020, max_per_query: 25 }
      vi.mocked(CreateProfile).mockResolvedValue(created as any)
      vi.mocked(ListProfiles).mockResolvedValue([created] as any)

      const store = useProfileStore()
      const result = await store.createProfile({ name: 'New' })

      expect(result.id).toBe(5)
      expect(store.activeProfileId).toBe(5)
      expect(store.profiles.length).toBe(1)
    })
  })

  describe('deleteProfile', () => {
    it('clears activeProfileId when deleting the active profile', async () => {
      vi.mocked(DeleteProfile).mockResolvedValue(undefined as any)
      vi.mocked(ListProfiles).mockResolvedValue([])

      const store = useProfileStore()
      store.activeProfileId = 1
      await store.deleteProfile(1)

      // After deletion, fetchProfiles returns [] → activeProfileId becomes null
      expect(store.activeProfileId).toBeNull()
      expect(store.profiles).toEqual([])
    })
  })
})
