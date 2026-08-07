import { defineStore } from 'pinia'
import { ref } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export interface SearchEvent {
  type: string
  axis: string
  query: string
  provider: string
  count: number
  total: number
  error: string
  duration_ms: number
  raw_count: number
}

export interface DownloadEvent {
  type: string
  paper_id: number
  title: string
  source: string
  url: string
  error: string
  current: number
  total: number
  filename: string
}

export type NotifyFn = (type: 'success' | 'error' | 'warning' | 'info', content: string) => void

export const useProgressStore = defineStore('progress', () => {
  const searching = ref(false)
  const downloading = ref(false)
  const radarRunning = ref(false)
  const reviewRunning = ref(false)
  const reviewCurrent = ref(0)
  const reviewTotal = ref(0)
  const reviewLabel = ref('')
  const searchEvents = ref<SearchEvent[]>([])
  const downloadEvents = ref<DownloadEvent[]>([])
  const current = ref(0)
  const total = ref(0)
  const lastEvent = ref<string>('')

  // Callbacks set by App.vue for cross-view notifications and data refresh.
  let notify: NotifyFn = () => {}
  let onSearchDone: (() => void) | null = null
  let onDownloadDone: (() => void) | null = null

  function setNotify(fn: NotifyFn) {
    notify = fn
  }

  function setOnSearchDone(fn: () => void) {
    onSearchDone = fn
  }

  function setOnDownloadDone(fn: () => void) {
    onDownloadDone = fn
  }

  function resetSearch() {
    searchEvents.value = []
    current.value = 0
    total.value = 0
    lastEvent.value = ''
  }

  function resetDownload() {
    downloadEvents.value = []
    current.value = 0
    total.value = 0
    lastEvent.value = ''
  }

  function startListening() {
    EventsOn('search:progress', (event: SearchEvent) => {
      searchEvents.value.push(event)
      if (event.type === 'provider_done') {
        lastEvent.value = `${event.provider}: ${event.count} papers`
      } else if (event.type === 'query_start') {
        lastEvent.value = `Searching: "${event.query}"`
      } else if (event.type === 'axis_done') {
        lastEvent.value = `Axis done: ${event.total} papers`
      } else if (event.type === 'provider_error') {
        lastEvent.value = `${event.provider}: error`
      }
    })

    EventsOn('search:done', (data: any) => {
      searching.value = false
      total.value = data?.total || 0
      lastEvent.value = `Search complete: ${total.value} papers`
      notify('success', `Search complete: found ${total.value} papers`)
      onSearchDone?.()
    })

    EventsOn('search:error', (data: any) => {
      searchEvents.value.push({
        type: 'error', axis: data?.axis || '', query: '', provider: '',
        count: 0, total: 0, error: data?.error || '', duration_ms: 0, raw_count: 0,
      })
      lastEvent.value = `Error: ${data?.axis}: ${data?.error}`
    })

    EventsOn('download:progress', (event: DownloadEvent) => {
      downloadEvents.value.push(event)
      current.value = event.current
      total.value = event.total
      if (event.type === 'start') {
        lastEvent.value = `[${event.current}/${event.total}] ${event.title}`
      } else if (event.type === 'resolving') {
        lastEvent.value = `[${event.current}/${event.total}] Trying ${event.source}...`
      } else if (event.type === 'done') {
        lastEvent.value = `[${event.current}/${event.total}] Downloaded via ${event.source}`
      } else if (event.type === 'fail' && event.error) {
        lastEvent.value = `[${event.current}/${event.total}] Failed: ${event.error}`
      }
    })

    EventsOn('download:done', () => {
      downloading.value = false
      const done = downloadEvents.value.filter(e => e.type === 'done').length
      const failed = downloadEvents.value.filter(e => e.type === 'fail').length
      lastEvent.value = 'Download complete'
      notify(
        failed > 0 ? 'warning' : 'success',
        `Download complete: ${done} downloaded, ${failed} failed`,
      )
      onDownloadDone?.()
    })

    EventsOn('review:progress', (data: any) => {
      reviewRunning.value = true
      reviewCurrent.value = data?.current ?? 0
      reviewTotal.value = data?.total ?? 0
      reviewLabel.value = data?.label ?? ''
    })

    EventsOn('review:done', () => {
      reviewRunning.value = false
      reviewCurrent.value = 0
      reviewTotal.value = 0
      reviewLabel.value = ''
    })

    EventsOn('radar:done', () => {
      radarRunning.value = false
    })
  }

  function stopListening() {
    EventsOff('search:progress', 'search:done', 'search:error', 'download:progress', 'download:done', 'review:progress', 'review:done', 'radar:done')
  }

  return {
    searching, downloading, radarRunning, searchEvents, downloadEvents,
    current, total, lastEvent, reviewRunning, reviewCurrent, reviewTotal, reviewLabel,
    resetSearch, resetDownload, startListening, stopListening,
    setNotify, setOnSearchDone, setOnDownloadDone,
  }
})
