import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  ListLLMProfiles,
  CreateLLMProfile,
  UpdateLLMProfile,
  DeleteLLMProfile,
  SetActiveLLMProfile,
  TestLLMProfile,
  TestLLMProfileDraft,
} from '../../wailsjs/go/app/App'
import { app, db } from '../../wailsjs/go/models'

export const useLLMProfilesStore = defineStore('llmProfiles', () => {
  const profiles = ref<app.LLMProfileView[]>([])
  const loading = ref(false)

  const activeProfile = computed(() =>
    profiles.value.find(p => p.is_active) || null,
  )
  const hasActiveProfile = computed(() => activeProfile.value !== null)

  async function fetch() {
    loading.value = true
    try {
      profiles.value = await ListLLMProfiles() || []
    } catch {
      profiles.value = []
    } finally {
      loading.value = false
    }
  }

  async function create(profile: db.LLMProfile, apiKey: string) {
    const view = await CreateLLMProfile(profile, apiKey)
    await fetch()
    return view
  }

  async function update(profile: db.LLMProfile, apiKey: string) {
    const view = await UpdateLLMProfile(profile, apiKey)
    await fetch()
    return view
  }

  async function remove(id: number) {
    await DeleteLLMProfile(id)
    await fetch()
  }

  async function setActive(id: number) {
    await SetActiveLLMProfile(id)
    await fetch()
  }

  async function test(id: number) {
    await TestLLMProfile(id)
  }

  async function testDraft(baseURL: string, apiKey: string, model: string) {
    await TestLLMProfileDraft(baseURL, apiKey, model)
  }

  return {
    profiles, loading, activeProfile, hasActiveProfile,
    fetch, create, update, remove, setActive, test, testDraft,
  }
})
