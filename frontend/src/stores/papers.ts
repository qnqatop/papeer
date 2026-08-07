import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  ListPapers,
  GetPaper,
  UpdatePaperStatus,
  BulkUpdateStatus,
  SetUserScore,
  SetPaperNotes,
  ApproveByScore,
} from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

export const usePapersStore = defineStore('papers', () => {
  const papers = ref<db.Paper[]>([])
  const total = ref(0)
  const loading = ref(false)
  const filter = ref<db.PaperFilter>(new db.PaperFilter({ profile_id: 0, limit: 50, offset: 0 }))

  async function fetchPapers() {
    if (!filter.value.profile_id) return
    loading.value = true
    try {
      const result = await ListPapers(filter.value)
      papers.value = result.papers || []
      total.value = result.total
    } finally {
      loading.value = false
    }
  }

  async function setStatus(id: number, status: string) {
    await UpdatePaperStatus(id, status)
    await fetchPapers()
  }

  async function bulkSetStatus(ids: number[], status: string) {
    await BulkUpdateStatus(ids, status)
    await fetchPapers()
  }

  async function setScore(id: number, score: number) {
    await SetUserScore(id, score)
    await fetchPapers()
  }

  async function setNotes(id: number, notes: string) {
    await SetPaperNotes(id, notes)
  }

  async function approveByScore(profileId: number, minScore: number) {
    const count = await ApproveByScore(profileId, minScore)
    await fetchPapers()
    return count
  }

  function setFilter(updates: Partial<db.PaperFilter>) {
    filter.value = new db.PaperFilter({ ...filter.value, ...updates, offset: 0 })
    fetchPapers()
  }

  function nextPage() {
    const off = (filter.value.offset || 0) + (filter.value.limit || 50)
    if (off < total.value) {
      filter.value = new db.PaperFilter({ ...filter.value, offset: off })
      fetchPapers()
    }
  }

  function prevPage() {
    const off = Math.max(0, (filter.value.offset || 0) - (filter.value.limit || 50))
    filter.value = new db.PaperFilter({ ...filter.value, offset: off })
    fetchPapers()
  }

  return {
    papers, total, loading, filter,
    fetchPapers, setStatus, bulkSetStatus, setScore, setNotes,
    approveByScore, setFilter, nextPage, prevPage,
  }
})
