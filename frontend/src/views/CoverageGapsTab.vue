<template>
  <n-space vertical :size="16">
    <n-card v-if="!profileStore.activeProfileId" :title="t('coverageGaps.title')">
      <n-empty :description="t('common.selectProfileFirst')" />
    </n-card>

    <template v-else>
      <!-- Toolbar -->
      <n-space align="center" justify="space-between" :wrap="true">
        <n-space align="center" :size="12">
          <n-text depth="3" style="font-size: 13px">{{ t('coverageGaps.minMentions') }}:</n-text>
          <n-select v-model:value="pendingMinMentions" :options="minMentionsOptions" size="small" style="width: 90px" />
          <n-button size="small" :loading="loading" @click="applyThreshold">{{ t('common.apply') }}</n-button>

          <n-text depth="3" style="font-size: 13px; margin-left: 12px">{{ t('coverageGaps.addToAxis') }}:</n-text>
          <n-select
            v-model:value="targetAxisId"
            :options="axisOptions"
            :placeholder="t('coverageGaps.noAxis')"
            clearable
            size="small"
            style="width: 200px"
          />
        </n-space>

        <n-button size="small" :disabled="gaps.length === 0" :loading="exporting" @click="exportCsv">
          {{ t('coverageGaps.exportCsv') }}
        </n-button>
      </n-space>

      <!-- Loading -->
      <n-card v-if="loading">
        <div style="display: flex; justify-content: center; padding: 48px 0">
          <n-spin size="large" />
        </div>
      </n-card>

      <!-- Empty states -->
      <n-card v-else-if="gaps.length === 0" :title="t('coverageGaps.title')">
        <n-empty :description="emptyDescription" />
      </n-card>

      <!-- Table -->
      <n-card v-else :title="t('coverageGaps.title')">
        <template #header-extra>
          <n-text depth="3" style="font-size: 12px">{{ t('coverageGaps.count', { count: gaps.length }) }}</n-text>
        </template>
        <n-data-table
          :columns="columns"
          :data="gaps"
          :row-key="(row: db.ExternalCitationWithMentions) => row.id"
          size="small"
          :bordered="false"
          max-height="calc(100vh - 340px)"
          virtual-scroll
        />
      </n-card>
    </template>
  </n-space>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import type { DataTableColumn } from 'naive-ui'
import {
  NCard, NEmpty, NSpace, NText, NSpin, NSelect, NButton, NDataTable,
  NTag, NPopover, NButtonGroup,
  useMessage,
} from 'naive-ui'
import {
  GetCoverageGaps, ExportGapsCSV, SaveExportFile, ResolveAndAddExternal,
  DownloadApproved, ListAxes, GetCitationGraph,
} from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'
import { useProfileStore } from '../stores/profile'
import { localizeBackendError, isInvalidEmailError } from '../utils/errors'
import { fetchApprovedDownloadedMap } from '../utils/paperMap'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const profileStore = useProfileStore()

const gaps = ref<db.ExternalCitationWithMentions[]>([])
const loading = ref(false)
const exporting = ref(false)
const citationStats = ref<db.CitationStats | null>(null)
const pendingMinMentions = ref(2)
const appliedMinMentions = ref(2)
const axes = ref<db.Axis[]>([])
const targetAxisId = ref<number | null>(null)
const paperMap = ref<Map<number, db.Paper>>(new Map())
const addingId = ref<number | null>(null)
const addedIds = ref<Set<number>>(new Set())

const minMentionsOptions = [2, 3, 5, 10].map(v => ({ label: String(v), value: v }))

const axisOptions = computed(() =>
  axes.value.map(a => ({ label: a.axis_key, value: a.id }))
)

const emptyDescription = computed(() => {
  const s = citationStats.value
  if (!s || s.papers_eligible === 0) return t('coverageGaps.emptyNoPapers')
  if (s.papers_processed === 0) return t('coverageGaps.emptyNeverFetched')
  return t('coverageGaps.emptyNoGaps', { min: appliedMinMentions.value })
})

function paperTitle(id: number): string {
  return paperMap.value.get(id)?.title ?? `#${id}`
}

function openPaper(id: number) {
  router.push({ path: '/papers', query: { paper_id: String(id) } })
}

const columns = computed<DataTableColumn<db.ExternalCitationWithMentions>[]>(() => [
  {
    title: t('coverageGaps.colTitle'),
    key: 'title',
    ellipsis: { tooltip: true },
    render(row) {
      return h('a', {
        href: row.s2_paper_id ? `https://www.semanticscholar.org/paper/${row.s2_paper_id}` : undefined,
        target: '_blank',
        style: 'color: inherit; text-decoration: none',
        title: t('citationGraph.openInS2'),
      }, row.title)
    },
  },
  { title: t('papers.colYear'), key: 'year', width: 60, align: 'center' },
  { title: t('papers.colCitations'), key: 'citation_count', width: 70, align: 'center' },
  {
    title: t('coverageGaps.colMentions'),
    key: 'mention_count',
    width: 90,
    align: 'center',
    sorter: (a, b) => a.mention_count - b.mention_count,
    defaultSortOrder: 'descend',
    render(row) {
      return h(NTag, { size: 'small', type: 'warning', round: true }, () => String(row.mention_count))
    },
  },
  {
    title: t('coverageGaps.colMentionedBy'),
    key: 'mentioned_by',
    width: 160,
    render(row) {
      if (!row.mentioned_by || row.mentioned_by.length === 0) return '—'
      return h(NPopover, { trigger: 'hover' }, {
        trigger: () => h(NTag, { size: 'small' }, () => t('coverageGaps.papersCount', { count: row.mentioned_by.length })),
        default: () => h('div', { style: 'max-width: 320px; display: flex; flex-direction: column; gap: 4px' },
          row.mentioned_by.map(id => h('a', {
            style: 'font-size: 12px; cursor: pointer; color: var(--text-link, #9880ff)',
            onClick: () => openPaper(id),
          }, paperTitle(id)))),
      })
    },
  },
  {
    title: t('coverageGaps.colActions'),
    key: 'actions',
    width: 220,
    render(row) {
      if (addedIds.value.has(row.id)) {
        return h(NTag, { size: 'small', type: 'success' }, () => t('coverageGaps.added'))
      }
      const busy = addingId.value === row.id
      return h(NButtonGroup, {}, () => [
        h(NButton, {
          size: 'tiny', type: 'success', secondary: true, loading: busy,
          onClick: () => addExternal(row, false),
        }, () => t('coverageGaps.addApprove')),
        h(NButton, {
          size: 'tiny', type: 'primary', secondary: true, loading: busy,
          onClick: () => addExternal(row, true),
        }, () => t('coverageGaps.addDownload')),
      ])
    },
  },
])

async function loadGaps() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  loading.value = true
  try {
    const [gapsResult, graph] = await Promise.all([
      GetCoverageGaps(pid, appliedMinMentions.value, 500),
      // Cheap way to read papers_processed/papers_eligible without a
      // dedicated stats endpoint — a very high threshold keeps the
      // external-node list (and thus the response) small.
      GetCitationGraph(pid, 999999).catch(() => null),
    ])
    gaps.value = gapsResult || []
    citationStats.value = graph?.stats ?? null
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
    gaps.value = []
  } finally {
    loading.value = false
  }
}

function applyThreshold() {
  appliedMinMentions.value = pendingMinMentions.value
  loadGaps()
}

async function loadAxes() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    axes.value = await ListAxes(pid) || []
  } catch {
    axes.value = []
  }
}

async function loadPaperMap() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  try {
    paperMap.value = await fetchApprovedDownloadedMap(pid)
  } catch {
    paperMap.value = new Map()
  }
}

async function addExternal(row: db.ExternalCitationWithMentions, andDownload: boolean) {
  const pid = profileStore.activeProfileId
  if (!pid) return
  addingId.value = row.id
  try {
    await ResolveAndAddExternal(pid, row.id, targetAxisId.value ?? null, 'approved')
    if (andDownload) {
      await DownloadApproved(pid)
      message.success(t('coverageGaps.addedAndDownloading'))
    } else {
      message.success(t('coverageGaps.addedApproved'))
    }
    addedIds.value.add(row.id)
  } catch (e: any) {
    if (isInvalidEmailError(e)) message.error(t('profiles.emailMissingForOps'))
    else message.error(localizeBackendError(e, t))
  } finally {
    addingId.value = null
  }
}

async function exportCsv() {
  const pid = profileStore.activeProfileId
  if (!pid) return
  exporting.value = true
  try {
    const content = await ExportGapsCSV(pid)
    await SaveExportFile('coverage_gaps.csv', content)
    message.success(t('coverageGaps.csvExported'))
  } catch (e: any) {
    message.error(localizeBackendError(e, t))
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  loadAxes()
  loadPaperMap()
  loadGaps()
})

watch(() => profileStore.activeProfileId, () => {
  targetAxisId.value = null
  addedIds.value = new Set()
  citationStats.value = null
  loadAxes()
  loadPaperMap()
  loadGaps()
})

defineExpose({ reload: loadGaps })
</script>
