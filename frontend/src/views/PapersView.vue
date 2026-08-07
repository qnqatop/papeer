<template>
  <n-card>
    <template v-if="!profileStore.activeProfile">
      <n-empty :description="t('common.selectProfileFirst')" />
    </template>

    <template v-else>
      <!-- Status segments -->
      <n-space justify="space-between" align="center" style="margin-bottom: 12px">
        <n-tabs v-model:value="statusSegment" type="segment" size="small" class="status-segments">
          <n-tab :name="''" :tab="`${t('v2.papers.segmentAll')} (${statusCounts.new + statusCounts.approved + statusCounts.rejected + statusCounts.downloaded})`" />
          <n-tab :name="'new'" :tab="`${t('v2.papers.segmentNew')} (${statusCounts.new})`" />
          <n-tab :name="'approved'" :tab="`${t('v2.papers.segmentApproved')} (${statusCounts.approved})`" />
          <n-tab :name="'rejected'" :tab="`${t('v2.papers.segmentRejected')} (${statusCounts.rejected})`" />
          <n-tab :name="'downloaded'" :tab="`${t('v2.papers.segmentDownloaded')} (${statusCounts.downloaded})`" />
        </n-tabs>
      </n-space>

      <!-- Toolbar -->
      <n-space align="center" :wrap="true" style="margin-bottom: 12px" data-onboarding="papers-filters">
        <n-input
          v-model:value="searchFilter"
          :placeholder="t('papers.searchTitle')"
          clearable
          size="small"
          style="width: 220px"
          @update:value="debouncedSearch"
        />
        <n-popover trigger="click" placement="bottom-start" :show-arrow="false">
          <template #trigger>
            <n-button size="small" :type="activeFilterCount > 0 ? 'primary' : 'default'">
              {{ t('papers.filters') }}
              <template v-if="activeFilterCount > 0">({{ activeFilterCount }})</template>
            </n-button>
          </template>
          <div style="padding: 16px; width: 360px">
            <n-space vertical :size="12">
              <div>
                <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 4px">{{ t('papers.allAxes') }}</n-text>
                <n-select v-model:value="axisFilter" :options="axisOptions" :placeholder="t('papers.allAxes')" clearable size="small" />
              </div>
              <div>
                <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 4px">{{ t('papers.colScore') }} (0–10)</n-text>
                <n-space :size="8">
                  <n-input-number v-model:value="minScoreFilter" :placeholder="t('papers.from')" :min="0" :max="10" clearable size="small" style="width: 100%" />
                  <n-input-number v-model:value="maxScoreFilter" :placeholder="t('papers.to')" :min="0" :max="10" clearable size="small" style="width: 100%" />
                </n-space>
              </div>
              <div>
                <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 4px">{{ t('papers.colYear') }}</n-text>
                <n-space :size="8">
                  <n-input-number v-model:value="yearFromFilter" :placeholder="t('papers.from')" :min="1900" :max="2100" clearable size="small" style="width: 100%" />
                  <n-input-number v-model:value="yearToFilter" :placeholder="t('papers.to')" :min="1900" :max="2100" clearable size="small" style="width: 100%" />
                </n-space>
              </div>
              <div>
                <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 4px">{{ t('papers.minCitations') }}</n-text>
                <n-input-number v-model:value="minCitationsFilter" :min="0" clearable size="small" style="width: 100%" />
              </div>
              <div v-if="allTags.length > 0">
                <n-text depth="3" style="font-size: 12px; display: block; margin-bottom: 4px">{{ t('tags.filterByTag') }}</n-text>
                <n-select v-model:value="tagFilter" :options="tagOptions" :placeholder="t('tags.allTags')" clearable size="small" />
              </div>
              <div>
                <n-checkbox v-model:checked="hasSummaryFilter">{{ t('v2.papers.hasSummary') }}</n-checkbox>
              </div>
              <n-button size="small" quaternary :disabled="activeFilterCount === 0" @click="clearAllFilters" style="width: 100%">
                {{ t('papers.clearFilters') }}
              </n-button>
            </n-space>
          </div>
        </n-popover>
        <n-select v-model:value="sortBy" :options="sortOptions" style="width: 160px" size="small" />
        <n-text depth="3" style="font-size: 13px">{{ t('papers.totalPapers', { count: store.total }) }}</n-text>
        <div style="margin-left: auto; display: flex; gap: 8px">
          <n-button size="small" :loading="exportingBibtex" :disabled="exportDisabled" @click="exportBibtex">
            {{ t('papers.exportBibtex') }}
          </n-button>
          <n-button size="small" :loading="exportingCsv" :disabled="exportDisabled" @click="exportCsv">
            {{ t('papers.exportCsv') }}
          </n-button>
        </div>
      </n-space>

      <!-- Split layout -->
      <div class="papers-split" :style="{ height: splitHeight }">
        <!-- Left: table 40% -->
        <div class="papers-split__left">
          <n-spin :show="store.loading">
            <n-data-table
              :columns="columnsCompact"
              :data="store.papers"
              :row-key="(row: db.Paper) => row.id"
              :row-class-name="rowClassName"
              :row-props="rowPropsV2"
              size="small"
              :bordered="false"
              style="font-size: 13px"
              max-height="calc(100vh - 280px)"
              virtual-scroll
            />
          </n-spin>
          <n-space justify="center" style="margin-top: 8px" v-if="store.total > pageSize">
            <n-pagination :page="currentPage" :page-count="pageCount" :page-size="pageSize" @update:page="goToPage" size="small" />
          </n-space>
          <!-- Hotkeys hint -->
          <span class="text-caption" style="display: block; text-align: right; margin-top: 4px; padding-right: 4px; text-transform: none; letter-spacing: normal">
            {{ t('v2.papers.shortcutsBar') }}
          </span>
        </div>
        <!-- Right: detail 60% -->
        <div class="papers-split__right">
          <PaperDetailPanel
            :paper="selectedPaper"
            :tags="allTags"
            @set-status="setStatus"
            @set-score="setScore"
            @save-notes="saveNotes"
            @save-tags="savePaperTags"
            @close="selectedPaper = null; selectedIndex = -1"
          />
        </div>
      </div>

      <!-- Download banner -->
      <div v-if="approvedNoPdfCount > 0 && !downloadBannerDismissed" class="download-banner">
        <n-space align="center" justify="space-between">
          <n-text style="font-size: 13px">{{ t('v2.papers.downloadBanner', { count: approvedNoPdfCount }) }}</n-text>
          <n-space :size="8">
            <n-button size="small" type="primary" @click="showDownloadModal = true">{{ t('v2.papers.downloadPdf') }}</n-button>
            <n-button size="small" quaternary @click="downloadBannerDismissed = true">✕</n-button>
          </n-space>
        </n-space>
      </div>

      <!-- Download modal -->
      <DownloadModal v-model:show="showDownloadModal" />
    </template>

  </n-card>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, h } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { DataTableColumn } from 'naive-ui'
import {
  NCard, NEmpty, NSpace, NButton, NButtonGroup, NSelect, NInput,
  NInputNumber, NDataTable, NSpin, NPagination, NTag, NText, NTab,
  NPopover, NCheckbox,
  NTabs,
  useMessage,
} from 'naive-ui'
import PaperDetailPanel from '../components/PaperDetailPanel.vue'
import DownloadModal from '../components/DownloadModal.vue'
import { useProfileStore } from '../stores/profile'
import { usePapersStore } from '../stores/papers'
import { useSummaryStore } from '../stores/summary'
import { useLLMProfilesStore } from '../stores/llmProfiles'
import { useOnboardingPhase } from '../composables/useOnboarding'
import { AxesWithPapers, ListTags, SetPaperTags, ListPapers, ExportBibTeX, ExportCSV, SaveExportFile, GetPaper } from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

const { t } = useI18n()
const vueRoute = useRoute()
const profileStore = useProfileStore()
const store = usePapersStore()
const summaryStore = useSummaryStore()
const llmStore = useLLMProfilesStore()
const message = useMessage()

// Onboarding phase 4: papers review
useOnboardingPhase(4, {
  precondition: () => store.papers.length > 0,
  getSteps: () => [
    {
      element: '[data-onboarding="papers-filters"]',
      popover: {
        title: t('onboarding.papers.filtersTitle'),
        description: t('onboarding.papers.filtersDescription'),
      },
    },
    {
      popover: {
        title: t('onboarding.papers.tableTitle'),
        description: t('onboarding.papers.tableDescription'),
      },
    },
    {
      popover: {
        title: t('onboarding.papers.shortcutsTitle'),
        description: t('onboarding.papers.shortcutsDescription'),
      },
    },
  ],
})

const showDownloadModal = ref(false)
const downloadBannerDismissed = ref(false)
const statusSegment = ref('')
const statusCounts = ref({ new: 0, approved: 0, rejected: 0, downloaded: 0 })
const approvedNoPdfCount = ref(0)

// Export approved/downloaded papers as BibTeX / CSV. Disabled when there is
// nothing to export (backend errors on an empty selection otherwise).
const exportingBibtex = ref(false)
const exportingCsv = ref(false)
const exportDisabled = computed(() => statusCounts.value.approved + statusCounts.value.downloaded === 0)

async function exportBibtex() {
  if (!profileStore.activeProfileId) return
  exportingBibtex.value = true
  try {
    const content = await ExportBibTeX(profileStore.activeProfileId)
    await SaveExportFile('papeer_export.bib', content)
    message.success(t('papers.bibtexExported'))
  } catch (e: any) {
    message.error(t('papers.failed', { error: e?.message || String(e) }))
  } finally {
    exportingBibtex.value = false
  }
}

async function exportCsv() {
  if (!profileStore.activeProfileId) return
  exportingCsv.value = true
  try {
    const content = await ExportCSV(profileStore.activeProfileId)
    await SaveExportFile('papeer_export.csv', content)
    message.success(t('papers.csvExported'))
  } catch (e: any) {
    message.error(t('papers.failed', { error: e?.message || String(e) }))
  } finally {
    exportingCsv.value = false
  }
}

const splitHeight = computed(() => 'calc(100vh - 280px)')

// Watch statusSegment to sync with filters
watch(statusSegment, (val) => {
  statusFilter.value = val || null
})

// Load status counts for segments (respects current filters except status)
async function loadStatusCounts() {
  if (!profileStore.activeProfileId) return
  const pid = profileStore.activeProfileId
  const base: any = {
    profile_id: pid,
    limit: 1,
    offset: 0,
    axis_id: axisFilter.value ?? undefined,
    min_score: minScoreFilter.value ?? 0,
    max_score: maxScoreFilter.value ?? 0,
    min_citations: minCitationsFilter.value ?? 0,
    year_from: yearFromFilter.value ?? 0,
    year_to: yearToFilter.value ?? 0,
    search: searchFilter.value || '',
    tag_ids: tagFilter.value != null ? [tagFilter.value] : [],
    has_summary: hasSummaryFilter.value || undefined,
  }
  try {
    const [newR, appR, rejR, dlR] = await Promise.all([
      ListPapers(new db.PaperFilter({ ...base, status: 'new' })),
      ListPapers(new db.PaperFilter({ ...base, status: 'approved' })),
      ListPapers(new db.PaperFilter({ ...base, status: 'rejected' })),
      ListPapers(new db.PaperFilter({ ...base, status: 'downloaded' })),
    ])
    statusCounts.value = {
      new: newR.total,
      approved: appR.total,
      rejected: rejR.total,
      downloaded: dlR.total,
    }
    approvedNoPdfCount.value = appR.total
  } catch { /* ignore */ }
}

// V2 compact columns (no status buttons, less width)
const columnsCompact = computed<DataTableColumn<db.Paper>[]>(() => [
  {
    title: t('papers.colScore'),
    key: 'pre_score',
    width: 60,
    align: 'center',
    render(row) {
      return h(NTag, { size: 'small', type: row.pre_score >= 5 ? 'success' : row.pre_score >= 3 ? 'warning' : 'default', round: true }, () => String(row.pre_score))
    },
  },
  {
    title: t('papers.colTitle'),
    key: 'title',
    ellipsis: { tooltip: true },
  },
  {
    title: t('papers.colYear'),
    key: 'year',
    width: 50,
    align: 'center',
  },
  {
    title: t('papers.colCitations'),
    key: 'citation_count',
    width: 50,
    align: 'center',
  },
])

function rowPropsV2(row: db.Paper) {
  return {
    style: 'cursor: pointer',
    onClick: () => openDetailV2(row),
  }
}

function openDetailV2(paper: db.Paper) {
  selectedIndex.value = store.papers.indexOf(paper)
  selectedPaper.value = paper
  llmStore.fetch()
  summaryStore.fetchForPaper(paper.id)
  removeMonitoringTag(paper)
}

// Deep-link support: Analysis (graph nodes, Topics clusters, Key Papers,
// Coverage Gaps, Recommendations) navigates here with ?paper_id=<id> to open
// a specific paper's detail panel directly, regardless of the current table
// filters/sort — the target paper may not even be on the current page.
async function openPaperById(id: number) {
  try {
    const paper = await GetPaper(id)
    if (paper) openDetailV2(paper)
  } catch {
    message.error(t('v2.papers.paperNotFound'))
  }
}

const pageSize = 50
const axisFilter = ref<number | null>(null)
const statusFilter = ref<string | null>(null)
const minScoreFilter = ref<number | null>(null)
const maxScoreFilter = ref<number | null>(null)
const yearFromFilter = ref<number | null>(null)
const yearToFilter = ref<number | null>(null)
const minCitationsFilter = ref<number | null>(null)
const minUserScoreFilter = ref<number | null>(null)
const searchFilter = ref('')
const sortBy = ref('score')
const selectedPaper = ref<db.Paper | null>(null)
const selectedIndex = ref(-1)
const axes = ref<db.Axis[]>([])
const allTags = ref<db.Tag[]>([])
const tagFilter = ref<number | null>(null)
const hasSummaryFilter = ref(false)

const tagOptions = computed(() =>
  allTags.value.map(t => ({ label: t.name, value: t.id }))
)
const activeFilterCount = computed(() => {
  let count = 0
  if (axisFilter.value != null) count++
  if (statusFilter.value) count++
  if (minScoreFilter.value != null || maxScoreFilter.value != null) count++
  if (yearFromFilter.value != null || yearToFilter.value != null) count++
  if (minCitationsFilter.value != null) count++
  if (minUserScoreFilter.value != null) count++
  if (tagFilter.value != null) count++
  if (hasSummaryFilter.value) count++
  return count
})

function clearAllFilters() {
  axisFilter.value = null
  statusFilter.value = null
  minScoreFilter.value = null
  maxScoreFilter.value = null
  yearFromFilter.value = null
  yearToFilter.value = null
  minCitationsFilter.value = null
  minUserScoreFilter.value = null
  tagFilter.value = null
  hasSummaryFilter.value = false
}


const monitoringTagID = computed(() => {
  const tag = allTags.value.find(t => t.name === 'monitoring')
  return tag ? tag.id : null
})

let searchTimeout: ReturnType<typeof setTimeout> | null = null

const sortOptions = computed(() => [
  { label: t('papers.sortScore'), value: 'score' },
  { label: t('papers.sortYear'), value: 'year' },
  { label: t('papers.sortCitations'), value: 'citations' },
  { label: t('papers.sortTitle'), value: 'title' },
])

const axisOptions = computed(() =>
  axes.value.map(a => ({ label: a.axis_key, value: a.id }))
)

const currentPage = computed(() =>
  Math.floor((store.filter.offset || 0) / pageSize) + 1
)

const pageCount = computed(() =>
  Math.ceil(store.total / pageSize)
)

function rowClassName(row: db.Paper) {
  const classes: string[] = []
  if (row.status === 'rejected') classes.push('paper-rejected')
  const idx = store.papers.indexOf(row)
  if (idx === selectedIndex.value) classes.push('paper-selected')
  return classes.join(' ')
}

async function removeMonitoringTag(paper: db.Paper) {
  if (!monitoringTagID.value || !paper.tags?.length) return
  const hasMonitoring = paper.tags.some(t => t.tag_id === monitoringTagID.value)
  if (!hasMonitoring) return
  const remaining = paper.tags.filter(t => t.tag_id !== monitoringTagID.value).map(t => t.tag_id)
  try {
    await SetPaperTags(paper.id, remaining)
    paper.tags = paper.tags.filter(t => t.tag_id !== monitoringTagID.value)
  } catch {
    // ignore — не критично
  }
}

async function setStatus(paper: db.Paper, status: string) {
  try {
    await store.setStatus(paper.id, status)
    paper.status = status
    loadStatusCounts()
  } catch (e: any) {
    message.error(t('papers.failed', { error: e }))
  }
}

async function setScore(paper: db.Paper, score: number) {
  try {
    await store.setScore(paper.id, score)
    paper.user_score = score
  } catch (e: any) {
    message.error(t('papers.failed', { error: e }))
  }
}

async function saveNotes(paper: db.Paper, notes: string) {
  if (notes === paper.notes) return
  try {
    await store.setNotes(paper.id, notes)
    paper.notes = notes
  } catch (e: any) {
    message.error(t('papers.failed', { error: e }))
  }
}

function applyFilters() {
  store.setFilter({
    profile_id: profileStore.activeProfileId!,
    axis_id: axisFilter.value ?? undefined,
    status: statusFilter.value ?? '',
    min_score: minScoreFilter.value ?? 0,
    max_score: maxScoreFilter.value ?? 0,
    min_citations: minCitationsFilter.value ?? 0,
    year_from: yearFromFilter.value ?? 0,
    year_to: yearToFilter.value ?? 0,
    min_user_score: minUserScoreFilter.value ?? 0,
    search: searchFilter.value,
    tag_ids: tagFilter.value != null ? [tagFilter.value] : [],
    has_summary: hasSummaryFilter.value || undefined,
    sort_by: sortBy.value,
    limit: pageSize,
  })
}

function debouncedSearch() {
  if (searchTimeout) clearTimeout(searchTimeout)
  searchTimeout = setTimeout(applyFilters, 300)
}

function goToPage(page: number) {
  store.filter = new db.PaperFilter({
    ...store.filter,
    offset: (page - 1) * pageSize,
  })
  store.fetchPapers()
}

async function loadAxes() {
  if (!profileStore.activeProfileId) return
  try {
    axes.value = await AxesWithPapers(profileStore.activeProfileId) || []
  } catch {
    // ignore
  }
}

async function loadTags() {
  if (!profileStore.activeProfileId) return
  try {
    allTags.value = await ListTags(profileStore.activeProfileId) || []
  } catch {
    allTags.value = []
  }
}

async function savePaperTags(tagIDs: number[]) {
  if (!selectedPaper.value) return
  try {
    await SetPaperTags(selectedPaper.value.id, tagIDs)
    // Refresh paper list to update tag badges
    store.fetchPapers()
    message.success(t('tags.tagsUpdated'))
  } catch (e: any) {
    message.error(t('papers.failed', { error: e }))
  }
}

// Update selected paper when list changes. Don't auto-select — user must click.
watch(() => store.papers, (papers) => {
  if (papers.length === 0) {
    selectedIndex.value = -1
    selectedPaper.value = null
    return
  }
  // Preserve selection if the previously selected paper is still in the list.
  if (selectedPaper.value) {
    const idx = papers.findIndex(p => p.id === selectedPaper.value!.id)
    if (idx >= 0) {
      selectedIndex.value = idx
      selectedPaper.value = papers[idx]
      return
    }
  }
  // Otherwise deselect.
  selectedIndex.value = -1
  selectedPaper.value = null
})

// Watch filters
watch([axisFilter, statusFilter, minScoreFilter, maxScoreFilter, yearFromFilter, yearToFilter, minCitationsFilter, minUserScoreFilter, tagFilter, hasSummaryFilter, sortBy], () => {
  applyFilters()
  loadStatusCounts()
})

// Refresh counts when download modal closes
watch(showDownloadModal, (val) => {
  if (!val) {
    loadStatusCounts()
    downloadBannerDismissed.value = false
  }
})

watch(() => profileStore.activeProfileId, () => {
  if (profileStore.activeProfileId) {
    loadAxes()
    loadTags()
    applyFilters()
    loadStatusCounts()
  }
})

// Keyboard shortcuts: j/k navigate, a approve, r reject, Enter open detail
function handleKeydown(e: KeyboardEvent) {
  // Skip if user is typing in an input or editable element
  const el = e.target as HTMLElement
  const tag = el.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable) return
  const papers = store.papers
  if (papers.length === 0) return

  switch (e.key) {
    case 'j':
    case 'ArrowDown': {
      e.preventDefault()
      if (selectedIndex.value < 0) selectedIndex.value = -1
      selectedIndex.value = Math.min(selectedIndex.value + 1, papers.length - 1)
      selectedPaper.value = papers[selectedIndex.value]
      scrollToSelected()
      break
    }
    case 'k':
    case 'ArrowUp': {
      e.preventDefault()
      if (selectedIndex.value < 0) selectedIndex.value = 1
      selectedIndex.value = Math.max(selectedIndex.value - 1, 0)
      selectedPaper.value = papers[selectedIndex.value]
      scrollToSelected()
      break
    }
    case 'a': {
      e.preventDefault()
      const paper = papers[selectedIndex.value]
      if (paper) setStatus(paper, paper.status === 'approved' ? 'new' : 'approved')
      break
    }
    case 'r': {
      e.preventDefault()
      const paper = papers[selectedIndex.value]
      if (paper) setStatus(paper, paper.status === 'rejected' ? 'new' : 'rejected')
      break
    }
    case 'Enter': {
      e.preventDefault()
      const paper = papers[selectedIndex.value]
      if (paper) openDetailV2(paper)
      break
    }
  }
}

function scrollToSelected() {
  const rows = document.querySelectorAll('.paper-selected')
  if (rows.length > 0) {
    rows[0].scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }
}

onMounted(() => {
  // Apply query params
  if (vueRoute.query.has_summary === 'true') {
    hasSummaryFilter.value = true
  }
  if (profileStore.activeProfileId) {
    loadAxes()
    loadTags()
    applyFilters()
    loadStatusCounts()
  }
  if (vueRoute.query.paper_id) {
    const id = Number(vueRoute.query.paper_id)
    if (!Number.isNaN(id)) openPaperById(id)
  }
  document.addEventListener('keydown', handleKeydown, true)
})

// Re-open when navigating here again with a different ?paper_id while
// already on this route (Vue Router doesn't remount in that case).
watch(() => vueRoute.query.paper_id, (v) => {
  if (!v) return
  const id = Number(v)
  if (!Number.isNaN(id)) openPaperById(id)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown, true)
})
</script>

<style>
.status-segments .n-tabs-tab {
  padding: 4px 14px !important;
  font-size: 13px !important;
  font-weight: 500;
}

.paper-rejected {
  opacity: 0.45;
}

.paper-selected td {
  background: rgba(124, 92, 255, 0.10) !important;
}

.paper-selected td:first-child {
  box-shadow: inset 3px 0 0 0 var(--color-primary-500, #7c5cff);
}

.papers-split {
  display: flex;
  gap: var(--space-4, 16px);
  overflow: hidden;
}

.papers-split__left {
  flex: 4;
  min-width: 0;
  overflow: hidden;
}

.papers-split__right {
  flex: 6;
  min-width: 0;
  overflow: hidden;
  border-left: 1px solid var(--surface-divider, rgba(148, 163, 184, 0.08));
  padding-left: var(--space-4, 16px);
}

.download-banner {
  border: 1px solid var(--surface-border, rgba(148, 163, 184, 0.12));
  border-radius: var(--radius-md, 8px);
  padding: var(--space-3, 12px) var(--space-4, 16px);
  margin-top: var(--space-3, 12px);
  background: var(--surface-elevated, #253347);
}

/* DataTable row hover */
.n-data-table .n-data-table-tr:hover > .n-data-table-td {
  background: rgba(124, 92, 255, 0.04) !important;
}
</style>
