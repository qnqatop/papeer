<template>
  <div ref="rootEl" class="pdf-viewer" :class="{ 'pdf-viewer--fullscreen': isFullscreen }">
    <!-- Loading -->
    <div v-if="loading" class="pdf-viewer__loading">
      <n-spin size="medium" />
      <n-text depth="3" style="margin-top: 8px">{{ t('papers.pdfLoading') }}</n-text>
      <n-progress
        v-if="loadProgress > 0 && loadProgress < 100"
        type="line"
        :percentage="loadProgress"
        style="max-width: 200px; margin-top: 8px"
      />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="pdf-viewer__error">
      <n-text type="error" style="font-size: 14px">{{ error }}</n-text>
      <n-space :size="8" style="margin-top: 12px">
        <n-button size="small" @click="loadPDF">{{ t('common.retry') }}</n-button>
        <n-button size="small" @click="$emit('close')">{{ t('common.cancel') }}</n-button>
      </n-space>
    </div>

    <!-- PDF content -->
    <template v-else-if="numPages > 0">
      <!-- Toolbar -->
      <div class="pdf-toolbar">
        <n-space align="center" :size="8">
          <n-button quaternary circle size="tiny" @click="emitClose">
            <template #icon><n-icon :component="ArrowBackOutline" size="16" /></template>
          </n-button>

          <n-divider vertical />

          <n-button quaternary circle size="tiny" :disabled="currentPage <= 1" @click="goToPage(currentPage - 1)">
            <template #icon><n-icon :component="ChevronBackOutline" size="16" /></template>
          </n-button>

          <n-input
            :value="String(currentPage)"
            size="tiny"
            style="width: 48px; text-align: center"
            @update:value="(v: string) => { const n = parseInt(v); if (n >= 1 && n <= numPages) goToPage(n) }"
          />

          <n-text depth="3" style="font-size: 12px; white-space: nowrap">/ {{ numPages }}</n-text>

          <n-button quaternary circle size="tiny" :disabled="currentPage >= numPages" @click="goToPage(currentPage + 1)">
            <template #icon><n-icon :component="ChevronForwardOutline" size="16" /></template>
          </n-button>

          <n-divider vertical />

          <n-select
            :value="zoomPreset"
            :options="zoomOptions"
            size="tiny"
            style="width: 110px"
            @update:value="setZoomPreset"
          />

          <n-button quaternary circle size="tiny" @click="adjustZoom(-0.1)">
            <template #icon><n-icon :component="RemoveOutline" size="14" /></template>
          </n-button>

          <n-button quaternary circle size="tiny" @click="adjustZoom(0.1)">
            <template #icon><n-icon :component="AddOutline" size="14" /></template>
          </n-button>

          <n-divider vertical />

          <n-button quaternary circle size="tiny" @click="toggleFullscreen" :title="t('papers.pdfFullscreen')">
            <template #icon>
              <n-icon :component="isFullscreen ? ContractOutline : ExpandOutline" size="16" />
            </template>
          </n-button>

          <n-button quaternary circle size="tiny" @click="downloadPDF" :title="t('papers.pdfDownload')">
            <template #icon><n-icon :component="DownloadOutline" size="16" /></template>
          </n-button>
        </n-space>
      </div>

      <!-- Canvas area -->
      <div ref="canvasContainer" class="pdf-canvas-area" @wheel="handleWheel">
        <div class="pdf-page-wrapper" :style="{ width: pageWidth + 'px' }">
          <canvas ref="pdfCanvas" class="pdf-canvas" />
        </div>
      </div>

      <!-- Status bar -->
      <div class="pdf-status">
        <n-text depth="3" style="font-size: 11px">
          {{ t('papers.pdfPageOf', { page: currentPage, total: numPages }) }}
          <template v-if="fileSizeMB > 0"> · {{ fileSizeMB }} MB</template>
          <template v-if="pdfSource"> · {{ pdfSource }}</template>
        </n-text>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onBeforeUnmount, onMounted, nextTick, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NSpin, NText, NSpace, NButton, NIcon, NDivider,
  NInput, NProgress, NSelect,
} from 'naive-ui'
import {
  ArrowBackOutline, ChevronBackOutline, ChevronForwardOutline,
  RemoveOutline, AddOutline, DownloadOutline,
  ExpandOutline, ContractOutline,
} from '@vicons/ionicons5'
import * as pdfjsLib from 'pdfjs-dist'
import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.mjs?url'

import { GetPaperPDFData } from '../../wailsjs/go/app/App'

// Set worker before any pdf.js calls
pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorkerUrl

const { t } = useI18n()

const props = defineProps<{
  paperId: number
  pdfSource?: string
  initialPage?: number
  initialZoom?: number
}>()

const emit = defineEmits<{
  close: []
  stateSave: [state: { page: number; zoom: number }]
}>()

// State
const loading = ref(true)
const loadProgress = ref(0)
const error = ref<string | null>(null)
const numPages = ref(0)
const currentPage = ref(props.initialPage ?? 1)
const zoom = ref(props.initialZoom ?? 0) // 0 = fit width
const fileSizeMB = ref(0)
const isFullscreen = ref(false)

const ZOOM_STORAGE_KEY = 'pdfViewer:zoomPreset'

// Refs
const rootEl = ref<HTMLElement | null>(null)
const canvasContainer = ref<HTMLElement | null>(null)
const pdfCanvas = ref<HTMLCanvasElement | null>(null)
const pageWidth = ref(800)

// pdf.js objects — stored outside reactivity to avoid proxy issues
let pdfDocument: any = null
let currentRenderTask: any = null

// Zoom preset system. Persist last user choice so reopening another PDF
// keeps the size the user has settled on.
const zoomPreset = ref(loadStoredZoomPreset())

function loadStoredZoomPreset(): string {
  try {
    const v = localStorage.getItem(ZOOM_STORAGE_KEY)
    if (v) return v
  } catch {}
  return 'fit-width'
}

function persistZoomPreset(v: string) {
  try { localStorage.setItem(ZOOM_STORAGE_KEY, v) } catch {}
}
const zoomOptions = computed(() => [
  { label: '50%', value: '0.5' },
  { label: '75%', value: '0.75' },
  { label: '100%', value: '1.0' },
  { label: '125%', value: '1.25' },
  { label: '150%', value: '1.5' },
  { label: t('papers.pdfFitWidth'), value: 'fit-width' },
  { label: t('papers.pdfFitPage'), value: 'fit-page' },
])

function setZoomPreset(value: string) {
  zoomPreset.value = value
  persistZoomPreset(value)
  if (value === 'fit-width' || value === 'fit-page') {
    zoom.value = 0
    renderCurrentPage()
  } else {
    zoom.value = parseFloat(value)
    zoomPreset.value = value
    renderCurrentPage()
  }
}

function adjustZoom(delta: number) {
  const newZoom = Math.max(0.25, Math.min(3.0, (zoom.value || getFitWidthScale()) + delta))
  zoom.value = Math.round(newZoom * 100) / 100
  zoomPreset.value = String(zoom.value)
  persistZoomPreset(zoomPreset.value)
  renderCurrentPage()
}

async function toggleFullscreen() {
  const el = rootEl.value
  if (!el) return
  try {
    if (!document.fullscreenElement) {
      await el.requestFullscreen()
    } else {
      await document.exitFullscreen()
    }
  } catch (e) {
    console.warn('fullscreen toggle failed', e)
  }
}

function onFullscreenChange() {
  isFullscreen.value = document.fullscreenElement === rootEl.value
  // After transition, container size changes — re-render at fit-* presets.
  if (zoomPreset.value === 'fit-width' || zoomPreset.value === 'fit-page') {
    nextTick(() => renderCurrentPage())
  }
}

function getFitWidthScale(): number {
  const container = canvasContainer.value
  if (!container) return 1.0
  return (container.clientWidth - 32) / pageWidth.value
}

function getFitPageScale(): number {
  const container = canvasContainer.value
  if (!container) return 1.0
  return (container.clientHeight - 80) / (pageWidth.value * 1.414) // assume A4-ish ratio
}

function getEffectiveZoom(): number {
  if (zoom.value > 0) return zoom.value
  if (zoomPreset.value === 'fit-page') return getFitPageScale()
  return getFitWidthScale()
}

async function loadPDF() {
  loading.value = true
  error.value = null
  loadProgress.value = 0

  try {
    // 1. Получаем файл напрямую через IPC-мост Wails
    const b64Data = await GetPaperPDFData(props.paperId)

    // 2. Быстро конвертируем Base64 в Uint8Array
    const binaryString = window.atob(b64Data)
    const len = binaryString.length
    const bytes = new Uint8Array(len)
    for (let i = 0; i < len; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }

    // 3. Скармливаем бинарные данные в pdf.js (обрати внимание на динамический cMapUrl)
    const loadingTask = pdfjsLib.getDocument({
      data: bytes,
      cMapUrl: `https://unpkg.com/pdfjs-dist@${pdfjsLib.version}/cmaps/`,
      cMapPacked: true,
    })

    loadingTask.onProgress = (progress: { loaded: number; total: number }) => {
      if (progress.total > 0) {
        loadProgress.value = Math.round((progress.loaded / progress.total) * 100)
        fileSizeMB.value = Math.round(progress.total / (1024 * 1024) * 10) / 10
      }
    }

    pdfDocument = await loadingTask.promise
    numPages.value = pdfDocument.numPages

    // Go to initial page (or page 1)
    if (props.initialPage && props.initialPage >= 1 && props.initialPage <= numPages.value) {
      currentPage.value = props.initialPage
    } else {
      currentPage.value = 1
    }

    if (props.initialZoom && props.initialZoom > 0) {
      zoom.value = props.initialZoom
      zoomPreset.value = String(props.initialZoom)
    } else if (zoomPreset.value !== 'fit-width' && zoomPreset.value !== 'fit-page') {
      // Restore numeric preset from localStorage.
      const parsed = parseFloat(zoomPreset.value)
      if (!isNaN(parsed) && parsed > 0) zoom.value = parsed
    }

    loading.value = false
    await nextTick()
    await renderCurrentPage()
  } catch (e: any) {
    loading.value = false
    const msg = e?.message || String(e)
    if (msg.includes('password') || msg.includes('encrypted')) {
      error.value = t('papers.pdfPassword')
    } else if (msg.includes('Invalid PDF') || msg.includes('corrupt')) {
      error.value = t('papers.pdfCorrupted')
    } else if (msg.includes('404') || msg.includes('not found') || msg.includes('Not Found') || msg.includes('no successful download')) {
      error.value = t('papers.pdfNotFound')
    } else {
      error.value = t('papers.pdfError', { error: msg })
    }
  }
}

async function renderCurrentPage() {
  if (!pdfDocument || !pdfCanvas.value) return

  // Cancel any in-flight render
  if (currentRenderTask) {
    currentRenderTask.cancel()
    currentRenderTask = null
  }

  try {
    const page = await pdfDocument.getPage(currentPage.value)
    const effectiveZoom = getEffectiveZoom()
    const viewport = page.getViewport({ scale: effectiveZoom })

    const canvas = pdfCanvas.value
    canvas.width = viewport.width
    canvas.height = viewport.height
    pageWidth.value = viewport.width

    const ctx = canvas.getContext('2d')!
    currentRenderTask = page.render({ canvasContext: ctx, viewport })
    await currentRenderTask.promise
    currentRenderTask = null
  } catch (e: any) {
    if (e?.name !== 'RenderingCancelledException') {
      console.error('PDF render error:', e)
    }
  }
}

function goToPage(pageNum: number) {
  if (pageNum < 1 || pageNum > numPages.value) return
  currentPage.value = pageNum
  renderCurrentPage()
}

function handleWheel(e: WheelEvent) {
  if (e.ctrlKey || e.metaKey) {
    e.preventDefault()
    const delta = e.deltaY > 0 ? -0.1 : 0.1
    adjustZoom(delta)
  }
}

function emitClose() {
  emit('stateSave', { page: currentPage.value, zoom: zoom.value || getFitWidthScale() })
  emit('close')
}

async function downloadPDF() {
  const link = document.createElement('a')
  link.href = `/api/pdf/${props.paperId}`
  link.download = `paper-${props.paperId}.pdf`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// Handle resize
let resizeObserver: ResizeObserver | null = null

function setupResizeObserver() {
  if (!canvasContainer.value) return
  resizeObserver = new ResizeObserver(() => {
    if (zoomPreset.value === 'fit-width' || zoomPreset.value === 'fit-page') {
      renderCurrentPage()
    }
  })
  resizeObserver.observe(canvasContainer.value)
}

onMounted(() => {
  loadPDF()
  nextTick(() => setupResizeObserver())
  document.addEventListener('fullscreenchange', onFullscreenChange)
})

watch(() => props.paperId, (newId, oldId) => {
  if (oldId) {
    if (pdfDocument) {
      pdfDocument.destroy()
      pdfDocument = null
    }
  }
  if (newId) loadPDF()
})

onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (currentRenderTask) {
    currentRenderTask.cancel()
    currentRenderTask = null
  }
  if (pdfDocument) {
    pdfDocument.destroy()
    pdfDocument = null
  }
})
</script>

<style scoped>
.pdf-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-neutral-700, #334155);
}

.pdf-viewer--fullscreen {
  width: 100vw;
  height: 100vh;
  background: var(--color-neutral-800, #1e293b);
}

.pdf-toolbar {
  flex-shrink: 0;
  padding: 4px 8px;
  border-bottom: 1px solid var(--surface-border, rgba(148, 163, 184, 0.12));
  background: var(--surface-card, #1e293b);
}

.pdf-canvas-area {
  flex: 1;
  overflow: auto;
  display: flex;
  justify-content: center;
  padding: 16px;
}

.pdf-page-wrapper {
  box-shadow: var(--shadow-lg, 0 4px 16px rgba(0, 0, 0, 0.4));
}

.pdf-canvas {
  display: block;
  background: white;
}

.pdf-status {
  flex-shrink: 0;
  padding: 4px 12px;
  border-top: 1px solid var(--surface-border, rgba(148, 163, 184, 0.12));
  background: var(--surface-card, #1e293b);
  text-align: center;
}

.pdf-viewer__loading,
.pdf-viewer__error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 300px;
  padding: 24px;
}
</style>
