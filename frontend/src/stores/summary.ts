import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  GenerateSummary,
  RegenerateSummary,
  GetSummaries,
  ListSummaries,
  DeleteSummary,
} from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

export const useSummaryStore = defineStore('summary', () => {
  const summariesByPaper = ref<Map<number, db.Summary[]>>(new Map())
  const allSummaries = ref<db.SummaryWithPaper[]>([])
  const generating = ref<Set<string>>(new Set())

  function genKey(paperID: number, model: string): string {
    return `${paperID}:${model}`
  }

  function startGenerating(key: string) {
    generating.value = new Set([...generating.value, key])
  }

  function stopGenerating(key: string) {
    const next = new Set(generating.value)
    next.delete(key)
    generating.value = next
  }

  function upsert(paperID: number, result: db.Summary) {
    const existing = summariesByPaper.value.get(paperID) || []
    const idx = existing.findIndex(s => s.model === result.model)
    if (idx >= 0) existing[idx] = result
    else existing.push(result)
    summariesByPaper.value.set(paperID, [...existing])
  }

  async function generate(paperID: number, model: string) {
    const key = genKey(paperID, model)
    if (generating.value.has(key)) return
    startGenerating(key)
    try {
      const result = await GenerateSummary(paperID, model)
      upsert(paperID, result)
      return result
    } finally {
      stopGenerating(key)
    }
  }

  async function regenerate(paperID: number, model: string) {
    const key = genKey(paperID, model)
    if (generating.value.has(key)) return
    startGenerating(key)
    try {
      const result = await RegenerateSummary(paperID, model)
      upsert(paperID, result)
      return result
    } finally {
      stopGenerating(key)
    }
  }

  async function fetchForPaper(paperID: number) {
    try {
      const list = await GetSummaries(paperID) || []
      summariesByPaper.value.set(paperID, list)
    } catch {
      summariesByPaper.value.set(paperID, [])
    }
  }

  async function fetchAll(profileID: number) {
    try {
      allSummaries.value = await ListSummaries(profileID) || []
    } catch {
      allSummaries.value = []
    }
  }

  async function remove(id: number, paperID: number) {
    await DeleteSummary(id)
    const existing = summariesByPaper.value.get(paperID) || []
    summariesByPaper.value.set(paperID, existing.filter(s => s.id !== id))
    allSummaries.value = allSummaries.value.filter(s => s.id !== id)
  }

  function isGenerating(paperID: number, model: string): boolean {
    return generating.value.has(genKey(paperID, model))
  }

  function hasSummary(paperID: number): boolean {
    const list = summariesByPaper.value.get(paperID)
    return (list?.length ?? 0) > 0
  }

  return {
    summariesByPaper, allSummaries, generating,
    generate, regenerate, fetchForPaper, fetchAll, remove,
    isGenerating, hasSummary, genKey,
  }
})
