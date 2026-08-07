import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  ListProfiles,
  GetProfile,
  CreateProfile,
  UpdateProfile,
  DeleteProfile,
  SelectDirectory,
} from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

export const useProfileStore = defineStore('profile', () => {
  const profiles = ref<db.Profile[]>([])
  const activeProfileId = ref<number | null>(null)
  const loading = ref(false)

  const activeProfile = computed(() =>
    profiles.value.find(p => p.id === activeProfileId.value) ?? null
  )

  async function fetchProfiles() {
    loading.value = true
    try {
      profiles.value = await ListProfiles() || []
      // Restore active profile if it still exists.
      if (activeProfileId.value && !profiles.value.find(p => p.id === activeProfileId.value)) {
        activeProfileId.value = profiles.value.length > 0 ? profiles.value[0].id : null
      }
      if (!activeProfileId.value && profiles.value.length > 0) {
        activeProfileId.value = profiles.value[0].id
      }
    } finally {
      loading.value = false
    }
  }

  async function createProfile(p: Partial<db.Profile>) {
    const created = await CreateProfile(new db.Profile(p))
    await fetchProfiles()
    activeProfileId.value = created.id
    return created
  }

  async function updateProfile(p: db.Profile) {
    await UpdateProfile(p)
    await fetchProfiles()
  }

  async function deleteProfile(id: number) {
    await DeleteProfile(id)
    await fetchProfiles()
  }

  async function selectPdfDir() {
    return await SelectDirectory('Select PDF download directory')
  }

  return {
    profiles, activeProfileId, activeProfile, loading,
    fetchProfiles, createProfile, updateProfile, deleteProfile, selectPdfDir,
  }
})
