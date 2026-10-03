import { defineStore } from 'pinia'
import { ref } from 'vue'
import { safeEventsOn } from '../composables/useWailsEvent'
import i18n from '../i18n'

const t = (key: string, params: Record<string, unknown> = {}) => i18n.global.t(key, params)

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

// Upper bound on retained progress events so long runs don't grow memory
// (and re-render cost of the logs) without limit.
export const MAX_PROGRESS_EVENTS = 500

function pushCapped<T>(list: T[], item: T) {
  list.push(item)
  if (list.length > MAX_PROGRESS_EVENTS) list.splice(0, list.length - MAX_PROGRESS_EVENTS)
}

// A paper whose download failed; kept until it is retried successfully or
// dismissed, independently of the capped per-run download log.
export interface FailedDownload {
  paper_id: number
  title: string
  error: string
  at: number
}

// One completed radar run as reported by the radar:done event.
export interface RadarRun {
  at: number
  profile_id: number
  new_papers: number
  processed: number
}

export const MAX_FAILED_DOWNLOADS = 50
export const MAX_RADAR_RUNS = 5

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
  // Totals for the current download run, kept separately because
  // downloadEvents is capped and may no longer hold every done/fail event.
  const downloadDone = ref(0)
  const downloadFailed = ref(0)
  const current = ref(0)
  const total = ref(0)
  const lastEvent = ref<string>('')
  const failedDownloads = ref<FailedDownload[]>([])
  const radarRuns = ref<RadarRun[]>([])

  function recordDownloadResult(event: DownloadEvent) {
    if (!event.paper_id) return
    const rest = failedDownloads.value.filter(f => f.paper_id !== event.paper_id)
    if (event.type === 'fail') {
      rest.unshift({ paper_id: event.paper_id, title: event.title, error: event.error || '', at: Date.now() })
      if (rest.length > MAX_FAILED_DOWNLOADS) rest.length = MAX_FAILED_DOWNLOADS
    }
    failedDownloads.value = rest
  }

  function dismissFailedDownload(paperId: number) {
    failedDownloads.value = failedDownloads.value.filter(f => f.paper_id !== paperId)
  }

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
    downloadDone.value = 0
    downloadFailed.value = 0
    current.value = 0
    total.value = 0
    lastEvent.value = ''
  }

  // Unsubscribe functions returned by EventsOn. stopListening calls them
  // instead of EventsOff(name), which would also drop other subscribers.
  let unsubs: Array<() => void> = []

  function listen(name: string, cb: (...data: any) => void) {
    const off = safeEventsOn(name, cb)
    if (typeof off === 'function') unsubs.push(off)
  }

  let listening = false

  function startListening() {
    if (listening) return
    listening = true
    listen('search:progress', (event: SearchEvent) => {
      pushCapped(searchEvents.value, event)
      if (event.type === 'provider_done') {
        lastEvent.value = t('progress.providerDone', { provider: event.provider, count: event.count })
      } else if (event.type === 'query_start') {
        lastEvent.value = t('progress.searching', { query: event.query })
      } else if (event.type === 'axis_done') {
        lastEvent.value = t('progress.axisDone', { count: event.total })
      } else if (event.type === 'provider_error') {
        lastEvent.value = t('progress.providerError', { provider: event.provider })
      } else if (event.type === 'save_error') {
        lastEvent.value = t('progress.saveError', { axis: event.axis, count: event.count })
      }
    })

    listen('search:done', (data: any) => {
      searching.value = false
      total.value = data?.total || 0
      lastEvent.value = t('progress.searchComplete', { count: total.value })
      notify('success', t('progress.searchCompleteToast', { count: total.value }))
      onSearchDone?.()
    })

    listen('search:error', (data: any) => {
      pushCapped(searchEvents.value, {
        type: 'error', axis: data?.axis || '', query: '', provider: '',
        count: 0, total: 0, error: data?.error || '', duration_ms: 0, raw_count: 0,
      })
      lastEvent.value = t('progress.searchError', { axis: data?.axis ?? '', error: data?.error ?? '' })
    })

    listen('download:progress', (event: DownloadEvent) => {
      if (!downloading.value) {
        resetDownload()
        downloading.value = true
      }
      pushCapped(downloadEvents.value, event)
      if (event.type === 'done') downloadDone.value++
      else if (event.type === 'fail') downloadFailed.value++
      // 'start' clears a stale failure row as soon as a retry begins;
      // a new 'fail' puts it back with the fresh reason.
      if (event.type === 'start' || event.type === 'done' || event.type === 'fail') recordDownloadResult(event)
      current.value = event.current
      total.value = event.total
      if (event.type === 'start') {
        lastEvent.value = `[${event.current}/${event.total}] ${event.title}`
      } else if (event.type === 'resolving') {
        lastEvent.value = t('progress.downloadTrying', { current: event.current, total: event.total, source: event.source })
      } else if (event.type === 'done') {
        lastEvent.value = t('progress.downloadDone', { current: event.current, total: event.total, source: event.source })
      } else if (event.type === 'fail' && event.error) {
        lastEvent.value = t('progress.downloadFailed', { current: event.current, total: event.total, error: event.error })
      }
    })

    listen('download:done', () => {
      downloading.value = false
      const done = downloadDone.value
      const failed = downloadFailed.value
      lastEvent.value = t('progress.downloadComplete')
      // Only show toast for batch downloads, not single-paper auto-downloads
      if (total.value > 1) {
        notify(
          failed > 0 ? 'warning' : 'success',
          t('progress.downloadCompleteToast', { done, failed }),
        )
      }
      onDownloadDone?.()
    })

    listen('review:progress', (data: any) => {
      reviewRunning.value = true
      reviewCurrent.value = data?.current ?? 0
      reviewTotal.value = data?.total ?? 0
      reviewLabel.value = data?.label ?? ''
    })

    listen('review:done', () => {
      reviewRunning.value = false
      reviewCurrent.value = 0
      reviewTotal.value = 0
      reviewLabel.value = ''
    })

    listen('radar:done', (data: any) => {
      radarRunning.value = false
      radarRuns.value = [
        {
          at: Date.now(),
          profile_id: data?.profile_id ?? 0,
          new_papers: data?.new_papers ?? 0,
          processed: data?.processed ?? 0,
        },
        ...radarRuns.value,
      ].slice(0, MAX_RADAR_RUNS)
    })
  }

  function stopListening() {
    for (const off of unsubs) off()
    unsubs = []
    listening = false
  }

  return {
    searching, downloading, radarRunning, searchEvents, downloadEvents, downloadDone, downloadFailed,
    current, total, lastEvent, failedDownloads, radarRuns, dismissFailedDownload, reviewRunning, reviewCurrent, reviewTotal, reviewLabel,
    resetSearch, resetDownload, startListening, stopListening,
    setNotify, setOnSearchDone, setOnDownloadDone,
  }
})
